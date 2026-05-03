package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Executor orchestrates flow step execution.
// It handles the step loop, condition evaluation, retry logic, fallback routing,
// compensation unwind, and the global on_error handler.
// Delegating normal step execution to a StepRunner.
type Executor struct {
	evaluator    ExpressionEvaluator
	stepExecutor StepExecutor
	stepRunner   StepRunner
}

// OnErrorExecutor is an optional interface that DSL step executors may implement
// to provide flow-level on_error and compensation handlers.
type OnErrorExecutor interface {
	ExecuteOnErrorHandler(execution *Execution, body string, fe *FlowError) error
	ExecuteCompensation(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error
}

func NewExecutor(evaluator ExpressionEvaluator, stepExecutor StepExecutor, stepRunner StepRunner) *Executor {
	return &Executor{
		evaluator:    evaluator,
		stepExecutor: stepExecutor,
		stepRunner:   stepRunner,
	}
}

// ExecuteSteps runs all steps in the flow for the given execution.
// The *Execution carries its own context (set by the HTTP handler with any
// flow-level timeout), so no separate ctx parameter is needed.
func (e *Executor) ExecuteSteps(execution *Execution) error {
	runCtx := &executionRunContext{flowFinalized: make(chan struct{})}
	defer close(runCtx.flowFinalized)

	if err := e.executeNodes(execution, NormalizeFlowNodes(execution.Flow), runCtx); err != nil {
		return err
	}

	if err := validateExecutionResponse(execution); err != nil {
		if handled, handlerErr := e.runOnErrorHandler(execution, err); handled {
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		}
		return err
	}

	return nil
}

type executionRunContext struct {
	flowFinalized    chan struct{}
	asyncWatcherOnce sync.Once
}

func (rc *executionRunContext) ensureAsyncWatcher(e *Executor, execution *Execution) {
	rc.asyncWatcherOnce.Do(func() {
		e.startAsyncCancellationWatcher(execution, rc.flowFinalized)
	})
}

type nodeExecutionResult struct {
	Next string
}

func (e *Executor) executeNodes(execution *Execution, nodes []FlowNode, runCtx *executionRunContext) error {
	nodeIndex, err := buildTopLevelNodeIndex(nodes)
	if err != nil {
		return e.handleFailure(execution, &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeRuntimeError),
			Message: err.Error(),
		})
	}

	for pc := 0; pc < len(nodes); {
		node := nodes[pc]
		if err := execution.Err(); err != nil {
			return e.handleFailure(execution, e.flowError(execution, node.ID, err))
		}

		result, fe := e.executeNode(execution, node, runCtx)
		if fe != nil {
			return e.handleFailure(execution, fe)
		}
		if execution.State().Response() != nil {
			execution.Logger().Info(fmt.Sprintf("Response produced at node: %s", node.ID))
			break
		}
		if result.Next != "" {
			target, ok := nodeIndex[result.Next]
			if !ok {
				return e.handleFailure(execution, runtimeErrorForInvalidNext(node.ID, result.Next))
			}
			if target <= pc {
				return e.handleFailure(execution, runtimeErrorForInvalidNext(node.ID, result.Next))
			}
			pc = target
			continue
		}
		pc++
	}
	return nil
}

func buildTopLevelNodeIndex(nodes []FlowNode) (map[string]int, error) {
	index := make(map[string]int, len(nodes))
	for i, node := range nodes {
		if node.ID == "" {
			return nil, fmt.Errorf("top-level flow node at index %d is missing an id", i)
		}
		if _, exists := index[node.ID]; exists {
			return nil, fmt.Errorf("duplicate top-level flow node id %q", node.ID)
		}
		index[node.ID] = i
	}
	return index, nil
}

func (e *Executor) executeNode(execution *Execution, node FlowNode, runCtx *executionRunContext) (nodeExecutionResult, *FlowError) {
	switch node.Kind {
	case FlowNodeStep:
		if node.Step == nil {
			return nodeExecutionResult{}, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("step node %q has no step", node.ID), Step: node.ID}
		}
		return e.executeStepNode(execution, *node.Step, runCtx)
	case FlowNodeParallel:
		if node.Parallel == nil {
			return nodeExecutionResult{}, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("parallel node %q has no block", node.ID), Step: node.ID}
		}
		return nodeExecutionResult{}, e.executeParallelBlock(execution, node, *node.Parallel, runCtx)
	default:
		return nodeExecutionResult{}, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("unsupported flow node kind %q", node.Kind), Step: node.ID}
	}
}

func (e *Executor) executeStepNode(execution *Execution, step Step, runCtx *executionRunContext) (nodeExecutionResult, *FlowError) {
	stepExec := execution.WithActiveStep(step.ID)
	if skip, err := e.evaluateCondition(stepExec, step); err != nil {
		return nodeExecutionResult{}, toFlowError(err, step.ID, 0)
	} else if skip {
		execution.Logger().Info(fmt.Sprintf("Skipping step (condition false): %s", step.ID))
		if step.Async {
			execution.AsyncTasks().Register(step.ID, NewCompletedAsyncTask(step.ID, nil, nil))
		}
		return nodeExecutionResult{}, nil
	}

	if step.Async {
		runCtx.ensureAsyncWatcher(e, execution)
		return nodeExecutionResult{}, e.spawnAsyncStep(execution, step, runCtx.flowFinalized)
	}

	output, fe, path := e.runStepPipeline(stepExec, step)
	if fe != nil {
		return nodeExecutionResult{}, fe
	}
	if output.Next != "" && !step.AllowsNext {
		return nodeExecutionResult{}, runtimeErrorForUserNext(step.ID)
	}

	applyStepOutput(execution, step.ID, output)
	if step.CompensateBody != "" {
		execution.State().AppendCompensation(CompensationEntry{
			StepID:   step.ID,
			Body:     step.CompensateBody,
			Path:     path,
			Compiled: step.CompensateCompiled,
		})
	}
	return nodeExecutionResult{Next: output.Next}, nil
}

func runtimeErrorForInvalidNext(stepID string, next string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: fmt.Sprintf("invalid __next target %q from %q", next, stepID),
		Step:    stepID,
	}
}

func runtimeErrorForUserNext(stepID string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: "user-authored __next is not allowed",
		Step:    stepID,
	}
}

func (e *Executor) runStepPipeline(execution *Execution, step Step) (StepOutput, *FlowError, SuccessPath) {
	log := execution.Logger()
	output, fe := e.executeStepWithRetries(execution, step, SuccessPathPrimary)
	if fe == nil {
		return output, nil, SuccessPathPrimary
	}
	if step.FallbackBody == "" {
		return StepOutput{}, fe, SuccessPathPrimary
	}

	log.Info(fmt.Sprintf("Primary failed for step %s, trying fallback", step.ID))
	fbStep := step
	fbStep.Body = step.FallbackBody
	fbStep.Compiled = step.FallbackCompiled
	fbStep.StoreKeys = step.FallbackStoreKeys
	fbStep.Retry = nil
	fbOutput, fbFE := e.executeStepWithRetriesWithExtra(execution, fbStep, SuccessPathFallback, map[string]any{
		"error": fe.ToMap(),
	})
	if fbFE != nil {
		log.Error(fmt.Sprintf("Fallback also failed for step %s", step.ID), "error", fbFE)
		return StepOutput{}, fbFE, SuccessPathFallback
	}
	return fbOutput, nil, SuccessPathFallback
}

// handleFailure runs compensation and on_error handling.
// Behavior:
//   - If no on_error is configured (or executable), returns original error.
//   - If on_error sets a response and does not raise, swallows the error.
//   - If on_error succeeds without a response, returns the original error.
//   - If on_error raises/returns an error, returns handler error instead of original.
func (e *Executor) handleFailure(execution *Execution, fe *FlowError) error {
	e.runCompensations(execution)
	handled, handlerErr := e.runOnErrorHandler(execution, fe)
	if !handled {
		return fe
	}
	if handlerErr != nil {
		return handlerErr
	}
	return nil
}

// HandleBoundaryError runs the flow-level on_error handler for an error raised
// before any step has executed, such as input contract validation.
func (e *Executor) HandleBoundaryError(execution *Execution, fe *FlowError) (bool, error) {
	handled, handlerErr := e.runOnErrorHandler(execution, fe)
	if handlerErr != nil {
		return handled, handlerErr
	}
	return handled, nil
}

func (e *Executor) spawnAsyncStep(execution *Execution, step Step, flowFinalized <-chan struct{}) *FlowError {
	asyncRuntime := asyncRuntimeForExecution(execution)
	if asyncRuntime == nil {
		return &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeRuntimeError),
			Message: "async runtime unavailable: execution has no container",
			Step:    step.ID,
		}
	}
	if err := asyncRuntime.Acquire(execution, execution.Metrics(), execFlowID(execution), step.ID); err != nil {
		return e.flowError(execution, step.ID, err)
	}

	snapshot := execution.State().Store().Snapshot()
	taskCtx, taskCancel := context.WithCancel(asyncRuntime.Context())
	spanCtx, span := execution.Tracer().Start(taskCtx, fmt.Sprintf("async step %s", step.ID),
		trace.WithAttributes(attribute.String("step.id", step.ID)))
	task := NewAsyncTask(step.ID, taskCancel, span)
	execution.AsyncTasks().Register(step.ID, task)
	execution.Metrics().RecordAsyncSpawn(spanCtx, execFlowID(execution), step.ID)

	isolatedExec := execution.
		WithIsolatedState(cloneRunStateSnapshot(snapshot)).
		WithContext(spanCtx).
		WithActiveStep(step.ID)

	go func() {
		fe := func() *FlowError {
			defer asyncRuntime.Release()
			output, fe, _ := e.runStepPipeline(isolatedExec, step)
			if fe != nil {
				task.Complete(nil, fe)
				return fe
			}
			task.Complete(output.Result, nil)
			return nil
		}()
		if fe != nil {
			<-flowFinalized
			if !task.Awaited() {
				execution.Logger().Error("Detached async step failed", "step", step.ID, "error", fe)
				execution.Metrics().RecordDetachedAsyncFailure(context.Background(), execFlowID(execution), step.ID, fe.Code)
			}
		}
	}()

	return nil
}

func (e *Executor) startAsyncCancellationWatcher(execution *Execution, flowFinalized <-chan struct{}) {
	asyncRuntime := asyncRuntimeForExecution(execution)
	if asyncRuntime == nil {
		return
	}
	go func() {
		select {
		case <-execution.Done():
			select {
			case <-flowFinalized:
				return
			default:
			}
			execution.AsyncTasks().CancelAll()
		case <-flowFinalized:
			return
		case <-asyncRuntime.Context().Done():
			return
		}
	}()
}

func asyncRuntimeForExecution(execution *Execution) *AsyncRuntime {
	if execution != nil && execution.Container != nil {
		return execution.Container.AsyncRuntime()
	}
	return nil
}

// executeStepWithRetries runs the step body respecting its RetryConfig.
// Returns the winning StepOutput on success or the last FlowError on exhausted retries.
func (e *Executor) executeStepWithRetries(execution *Execution, step Step, path SuccessPath) (StepOutput, *FlowError) {
	return e.executeStepWithRetriesWithExtra(execution, step, path, nil)
}

func (e *Executor) executeStepWithRetriesWithExtra(execution *Execution, step Step, path SuccessPath, extra map[string]any) (StepOutput, *FlowError) {
	parentCtx := execution.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	start := time.Now()
	var lastFE *FlowError
	var zero StepOutput

	spanCtx, span := execution.Tracer().Start(parentCtx, fmt.Sprintf("step %s", step.ID),
		trace.WithAttributes(
			attribute.String("step.id", step.ID),
		),
	)
	defer span.End()
	defer func() {
		execution.Metrics().RecordStep(
			spanCtx,
			execFlowID(execution),
			step.ID,
			string(path),
			classifyMetricOutcome(lastFlowError(lastFE)),
			time.Since(start),
		)
	}()

	stepCtx := spanCtx
	cancel := func() {}
	if step.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(spanCtx, time.Duration(step.Timeout)*time.Millisecond)
	}
	defer cancel()

	maxAttempts := 1
	if step.Retry != nil && step.Retry.MaxAttempts > 1 {
		maxAttempts = step.Retry.MaxAttempts
	}

	log := execution.Logger()

attemptLoop:
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Context check before each attempt.
		if ctxErr := stepCtx.Err(); ctxErr != nil {
			lastFE = e.flowError(execution, step.ID, ctxErr)
			lastFE.Retries = attempt
			break
		}

		// Wait between retries (skip for first attempt).
		if attempt > 0 && step.Retry != nil && step.Retry.Delay > 0 {
			delay := e.computeDelay(step.Retry, attempt)
			select {
			case <-time.After(delay):
			case <-stepCtx.Done():
				lastFE = e.flowError(execution, step.ID, stepCtx.Err())
				lastFE.Retries = attempt
				break attemptLoop
			}
		}

		stepExec := execution.WithContext(stepCtx).WithActivePath(path).WithActiveStep(step.ID)
		input, err := BuildStepInputWithExtra(execution, step, path, extra)
		if err != nil {
			lastFE = toFlowError(err, step.ID, attempt)
			break
		}
		output, err := e.stepRunner.RunStep(stepCtx, stepExec, input)
		if err == nil {
			lastFE = nil
			return output, nil
		}

		// Convert to FlowError.
		fe := toFlowError(err, step.ID, attempt)
		fe.Retries = attempt
		lastFE = fe

		log.Error(fmt.Sprintf("Step %s failed (attempt %d/%d)", step.ID, attempt+1, maxAttempts),
			"error_type", fe.Type,
			"error_code", fe.Code,
			"error", fe.Message)

		// Decide whether to retry.
		if attempt+1 < maxAttempts && e.shouldRetry(execution, step, fe) {
			execution.Metrics().RecordRetry(spanCtx, execFlowID(execution), step.ID, string(path))
			log.Info(fmt.Sprintf("Will retry step %s (attempt %d/%d)", step.ID, attempt+1, maxAttempts))
			continue
		}
		break
	}

	if lastFE != nil {
		span.RecordError(lastFE)
		span.SetStatus(codes.Error, lastFE.Message)
		span.SetAttributes(
			attribute.String("error.type", string(lastFE.Type)),
			attribute.String("error.code", lastFE.Code),
			attribute.Int("retries", lastFE.Retries),
		)
	}

	return zero, lastFE
}

func applyStepOutput(execution *Execution, stepID string, output StepOutput) {
	if output.Result != nil {
		if m, ok := output.Result.(map[string]any); ok {
			execution.State().Store().SetNested(stepID, m)
		} else {
			execution.State().Store().Set(stepID, output.Result)
		}
	}
	if output.Response != nil {
		execution.State().SetResponse(output.Response)
	}
}

func lastFlowError(fe *FlowError) error {
	if fe == nil {
		return nil
	}
	return fe
}

// shouldRetry decides whether to retry based on the RetryConfig and the error.
func (e *Executor) shouldRetry(execution *Execution, step Step, fe *FlowError) bool {
	retry := step.Retry
	if retry == nil {
		return false
	}

	// Non-retryable codes take precedence.
	if slices.Contains(retry.NonRetryable, fe.Code) {
		return false
	}

	// If a `when` expression is set, evaluate it with `error` injected into the store.
	if retry.When != "" {
		result, err := e.evaluator.EvalWithEnv(execution, retry.When, map[string]any{
			"error": fe.ToMap(),
		})
		if err != nil {
			execution.Logger().Error("error evaluating retry when expression", "error", err)
			return false
		}
		b, ok := result.(bool)
		return ok && b
	}

	// No expression: retry only transient errors.
	return fe.Type == ErrorTypeTransient
}

// computeDelay calculates the sleep duration for a retry attempt.
func (e *Executor) computeDelay(retry *RetryConfig, attempt int) time.Duration {
	base := time.Duration(retry.Delay) * time.Millisecond

	var delay time.Duration
	switch retry.Backoff {
	case "linear":
		delay = time.Duration(attempt) * base
	case "exponential":
		delay = time.Duration(math.Pow(2, float64(attempt-1))) * base
	default: // "none" or empty
		delay = base
	}

	if retry.MaxDelay > 0 {
		max := time.Duration(retry.MaxDelay) * time.Millisecond
		if delay > max {
			delay = max
		}
	}

	if retry.Jitter && delay > 0 {
		// Add up to 10% random jitter.
		jitter := time.Duration(rand.Int64N(int64(delay) / 10))
		delay += jitter
	}

	return delay
}

// runCompensations iterates the CompensationStack in LIFO order and executes
// each compensation body. Failures are logged but do not stop remaining compensations.
// Uses a detached context so compensation DB/HTTP calls complete even if the flow
// context was already cancelled (e.g. by a timeout).
func (e *Executor) runCompensations(execution *Execution) {
	oee, ok := e.stepExecutor.(OnErrorExecutor)
	if !ok {
		return
	}

	safeExec := execution.WithContext(context.WithoutCancel(execution))
	log := safeExec.Logger()
	stack := execution.State().CompensationSnapshot()
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		log.Info(fmt.Sprintf("Running compensation for step %s (path: %s)", entry.StepID, entry.Path))
		if err := e.runIsolatedCompensation(safeExec, oee, entry); err != nil {
			log.Error(fmt.Sprintf("Compensation failed for step %s", entry.StepID), "error", err)
			// Continue remaining compensations even on failure.
			continue
		}
	}
}

// runOnErrorHandler executes the flow-level on_error body if one is defined.
// Uses a detached context so the handler can complete (set response, update DB, etc.)
// even when the original flow context has already been cancelled by a timeout.
func (e *Executor) runOnErrorHandler(execution *Execution, fe *FlowError) (handled bool, handlerErr *FlowError) {
	if execution.Flow.OnErrorBody == "" {
		return false, nil
	}
	if _, ok := e.stepExecutor.(OnErrorExecutor); !ok {
		return false, nil
	}

	safeCtx := context.WithoutCancel(execution)
	log := execution.Logger()
	log.Info("Running flow-level on_error handler", "error_code", fe.Code)
	safeExec := execution.WithContext(safeCtx)
	output, err, handled := e.runIsolatedOnError(safeExec, fe)
	if !handled {
		return false, nil
	}
	if err != nil {
		log.Error("on_error handler itself failed", "error", err)
		return true, err
	}
	if responseErr := validateRecoveryResponse(execution, output.Response); responseErr != nil {
		return true, responseErr
	}
	applyOnErrorOutput(execution, output)
	return true, nil
}

func validateExecutionResponse(execution *Execution) *FlowError {
	return validateResponse(execution, execution.State().Response())
}

func validateRecoveryResponse(execution *Execution, response *ResponseDescriptor) *FlowError {
	return validateResponse(execution, response)
}

func validateResponse(execution *Execution, response *ResponseDescriptor) *FlowError {
	contract := responseContractForExecution(execution)
	if contract.IsZero() {
		return nil
	}
	if response == nil {
		if contract.RequiresResponse {
			return responseContractError("missing_response", "flow completed without a required response")
		}
		return nil
	}
	subtype := responseSubtype(response)
	if !contract.HasSubtype(subtype) {
		return responseContractError("invalid_response", fmt.Sprintf("response.%s is not valid for entrypoint.%s", subtype, contract.EntrypointType))
	}
	return nil
}

func responseContractForExecution(execution *Execution) ResponseContract {
	if execution == nil || execution.Flow == nil {
		return ResponseContract{}
	}
	if !execution.Flow.ResponseContract.IsZero() {
		return execution.Flow.ResponseContract
	}
	return ResponseContract{}
}

func responseSubtype(response *ResponseDescriptor) string {
	if response == nil {
		return ""
	}
	if response.Subtype != "" {
		return response.Subtype
	}
	if dot := strings.LastIndex(response.HandlerName, "."); dot >= 0 && dot+1 < len(response.HandlerName) {
		return response.HandlerName[dot+1:]
	}
	return response.HandlerName
}

func responseContractError(reason string, message string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: message,
		Meta: map[string]any{
			"reason": reason,
		},
	}
}

func (e *Executor) runIsolatedOnError(execution *Execution, fe *FlowError) (RecoveryOutput, *FlowError, bool) {
	oee := e.stepExecutor.(OnErrorExecutor)

	isolatedExec := execution.WithIsolatedState(cloneRunStateSnapshot(execution.State().Store().Snapshot()))
	err := oee.ExecuteOnErrorHandler(isolatedExec, execution.Flow.OnErrorBody, fe)
	if err != nil {
		return RecoveryOutput{}, toFlowError(err, "on_error", 0), true
	}

	return collectRecoveryOutput(isolatedExec.State()), nil, true
}

func (e *Executor) runIsolatedCompensation(execution *Execution, oee OnErrorExecutor, entry CompensationEntry) error {
	isolatedExec := execution.WithIsolatedState(cloneRunStateSnapshot(execution.State().Store().Snapshot()))
	return oee.ExecuteCompensation(isolatedExec, entry.Body, entry.StepID, entry.Path, entry.Compiled)
}

func applyOnErrorOutput(execution *Execution, output RecoveryOutput) {
	if output.Response != nil {
		execution.State().SetResponse(output.Response)
	}
	mergeStoreSnapshot(execution.State().Store(), output.Store)
}

func collectRecoveryOutput(state *RunState) RecoveryOutput {
	return RecoveryOutput{
		Response: state.Response(),
		Store:    state.Store().Snapshot(),
	}
}

func cloneRunStateSnapshot(snapshot map[string]any) *RunState {
	store := NewValueStore()
	mergeStoreSnapshot(store, snapshot)
	return NewRunState(store)
}

func mergeStoreSnapshot(store ValueStore, snapshot map[string]any) {
	for key, value := range snapshot {
		store.SetNested(key, value)
	}
}

// evaluateCondition returns (skip=true, nil) when the condition is false,
// (skip=false, nil) when it passes, or (false, err) on evaluation failure.
func (e *Executor) evaluateCondition(execution *Execution, step Step) (skip bool, err error) {
	if step.Condition == "" {
		return false, nil
	}

	log := execution.Logger()
	result, err := e.evaluator.Eval(execution, step.Condition)
	if err != nil {
		log.Error(fmt.Sprintf("Error evaluating condition for step %s", step.ID),
			"condition", step.Condition, "error", err)
		return false, fmt.Errorf("error evaluating condition %s: %w", step.Condition, err)
	}

	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("condition %s evaluated to %T, expected boolean", step.Condition, result)
	}
	if !b {
		return true, nil
	}
	log.Info(fmt.Sprintf("Condition met: %s", step.Condition))
	return false, nil
}

// flowError wraps a context error as a FlowError for unified error handling.
func (e *Executor) flowError(_ *Execution, stepID string, err error) *FlowError {
	if errors.Is(err, context.DeadlineExceeded) {
		return &FlowError{Type: ErrorTypeTimeout, Code: string(ErrorCodeDeadlineExceeded), Message: err.Error(), Step: stepID}
	}
	return &FlowError{Type: ErrorTypeTimeout, Code: string(ErrorCodeContextCancelled), Message: err.Error(), Step: stepID}
}

// toFlowError converts any error to a *FlowError, preserving existing FlowErrors.
func toFlowError(err error, stepID string, attempt int) *FlowError {
	if errors.Is(err, context.DeadlineExceeded) {
		return &FlowError{
			Type:    ErrorTypeTimeout,
			Code:    string(ErrorCodeDeadlineExceeded),
			Message: err.Error(),
			Step:    stepID,
			Retries: attempt,
		}
	}
	if errors.Is(err, context.Canceled) {
		return &FlowError{
			Type:    ErrorTypeTimeout,
			Code:    string(ErrorCodeContextCancelled),
			Message: err.Error(),
			Step:    stepID,
			Retries: attempt,
		}
	}

	var fe *FlowError
	if errors.As(err, &fe) {
		if fe.Step == "" {
			fe.Step = stepID
		}
		return fe
	}
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: err.Error(),
		Step:    stepID,
		Retries: attempt,
	}
}
