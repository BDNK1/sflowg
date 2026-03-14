package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Executor orchestrates flow step execution.
// It handles the step loop, condition evaluation, retry logic, fallback routing,
// compensation unwind, and the global on_error handler.
// Delegating actual step execution to a StepExecutor.
type Executor struct {
	evaluator    ExpressionEvaluator
	stepExecutor StepExecutor
}

// OnErrorExecutor is an optional interface that DSL step executors may implement
// to provide flow-level on_error and compensation handlers.
type OnErrorExecutor interface {
	ExecuteOnErrorHandler(execution *Execution, body string, fe *FlowError) error
	ExecuteCompensation(execution *Execution, body string, stepID string, path SuccessPath) error
}

func NewExecutor(evaluator ExpressionEvaluator, stepExecutor StepExecutor) *Executor {
	return &Executor{
		evaluator:    evaluator,
		stepExecutor: stepExecutor,
	}
}

// ExecuteSteps runs all steps in the flow for the given execution.
// The *Execution carries its own context (set by the HTTP handler with any
// flow-level timeout), so no separate ctx parameter is needed.
func (e *Executor) ExecuteSteps(execution *Execution) error {
	log := execution.Logger()
	nextStep := ""

	for _, s := range execution.Flow.Steps {
		// Check context before starting each step.
		if err := execution.Err(); err != nil {
			fe := e.flowError(execution, s.ID, err)
			return e.handleFailure(execution, fe)
		}

		// Step sequencing / branching.
		if nextStep != "" {
			if s.ID != nextStep {
				log.Info(fmt.Sprintf("Skipping step: %s", s.ID))
				continue
			}
			nextStep = ""
			log.Info(fmt.Sprintf("Resuming flow at step: %s", s.ID))
		}

		// Step condition guard.
		if skip, err := e.evaluateCondition(execution, s); err != nil {
			fe := toFlowError(err, s.ID, 0)
			return e.handleFailure(execution, fe)
		} else if skip {
			log.Info(fmt.Sprintf("Skipping step (condition false): %s", s.ID))
			continue
		}

		// --- Primary body with retries ---
		fe := e.executeStepWithRetries(execution, s, SuccessPathPrimary)

		if fe == nil {
			// Primary succeeded.
			// Propagate any results stored in the step-scoped execution back
			// to the parent (WithContext creates a shallow copy sharing the Store).
			if s.CompensateBody != "" {
				execution.CompensationStack = append(execution.CompensationStack, CompensationEntry{
					StepID: s.ID,
					Body:   s.CompensateBody,
					Path:   SuccessPathPrimary,
				})
			}
		} else {
			// Primary failed — try fallback if available.
			if s.FallbackBody != "" {
				log.Info(fmt.Sprintf("Primary failed for step %s, trying fallback", s.ID))
				fbStep := s
				fbStep.Body = s.FallbackBody
				fbStep.Retry = nil // fallback has no retry policy
				fbFE := e.executeStepWithRetries(execution, fbStep, SuccessPathFallback)

				if fbFE == nil {
					// Fallback succeeded — store its result under the original step ID
					// so downstream steps and compensation code use a stable key.
					if s.CompensateBody != "" {
						execution.CompensationStack = append(execution.CompensationStack, CompensationEntry{
							StepID: s.ID,
							Body:   s.CompensateBody,
							Path:   SuccessPathFallback,
						})
					}
					fe = nil // mark as success
				} else {
					// Both primary and fallback failed.
					log.Error(fmt.Sprintf("Fallback also failed for step %s", s.ID), "error", fbFE)
					return e.handleFailure(execution, fbFE)
				}
			} else {
				// No fallback — propagate error.
				return e.handleFailure(execution, fe)
			}
		}

		// Early exit if a step set a response.
		if execution.ResponseDescriptor != nil {
			log.Info(fmt.Sprintf("Response produced at step: %s", s.ID))
			break
		}

		if next := execution.Value(s.ID + ".__next"); next != nil {
			nextStep = fmt.Sprintf("%v", next)
		}
	}

	return nil
}

// handleFailure runs compensation and on_error handling.
// Behavior:
//   - If no on_error is configured (or executable), returns original error.
//   - If on_error succeeds without raising, swallows the error (returns nil).
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

// executeStepWithRetries runs the step body respecting its RetryConfig.
// Returns nil on success or the last FlowError on exhausted retries.
func (e *Executor) executeStepWithRetries(execution *Execution, step Step, path SuccessPath) *FlowError {
	parentCtx := execution.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	start := time.Now()
	var lastFE *FlowError

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

		var err error
		execution.WithScopedContext(stepCtx, func() {
			execution.WithActiveStep(step.ID, func() {
				_, err = e.stepExecutor.ExecuteStep(stepCtx, execution, step)
			})
		})
		if err == nil {
			lastFE = nil
			return nil
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

	return lastFE
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
		execution.Store.Set("error", fe.ToMap())
		result, err := e.evaluator.Eval(execution, retry.When)
		execution.Store.Set("error", nil) // clean up regardless of result
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
	stack := execution.CompensationStack
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		log.Info(fmt.Sprintf("Running compensation for step %s (path: %s)", entry.StepID, entry.Path))
		if err := oee.ExecuteCompensation(safeExec, entry.Body, entry.StepID, entry.Path); err != nil {
			log.Error(fmt.Sprintf("Compensation failed for step %s", entry.StepID), "error", err)
			// Continue remaining compensations even on failure.
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
	oee, ok := e.stepExecutor.(OnErrorExecutor)
	if !ok {
		return false, nil
	}

	safeCtx := context.WithoutCancel(execution)
	log := execution.Logger()
	log.Info("Running flow-level on_error handler", "error_code", fe.Code)
	var err error
	execution.WithScopedContext(safeCtx, func() {
		err = oee.ExecuteOnErrorHandler(execution, execution.Flow.OnErrorBody, fe)
		if err != nil {
			log.Error("on_error handler itself failed", "error", err)
		}
	})
	if err == nil {
		return true, nil
	}
	return true, toFlowError(err, "on_error", 0)
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
