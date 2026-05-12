package runtime

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

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

		fe := toFlowError(err, step.ID, attempt)
		fe.Retries = attempt
		lastFE = fe

		log.Error(fmt.Sprintf("Step %s failed (attempt %d/%d)", step.ID, attempt+1, maxAttempts),
			"error_type", fe.Type,
			"error_code", fe.Code,
			"error", fe.Message)

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

	if slices.Contains(retry.NonRetryable, fe.Code) {
		return false
	}

	if retry.When != "" {
		result, err := e.evaluator.EvalExpression(execution, retry.When, nil, nil, map[string]any{
			"error": fe.ToMap(),
		})
		if err != nil {
			execution.Logger().Error("error evaluating retry when expression", "error", err)
			return false
		}
		b, ok := result.(bool)
		return ok && b
	}

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
	default:
		delay = base
	}

	if retry.MaxDelay > 0 {
		max := time.Duration(retry.MaxDelay) * time.Millisecond
		if delay > max {
			delay = max
		}
	}

	if retry.Jitter && delay > 0 {
		jitter := time.Duration(rand.Int64N(int64(delay) / 10))
		delay += jitter
	}

	return delay
}

// evaluateCondition returns (skip=true, nil) when the condition is false,
// (skip=false, nil) when it passes, or (false, err) on evaluation failure.
func (e *Executor) evaluateCondition(execution *Execution, step Step) (skip bool, err error) {
	if step.Condition == "" {
		return false, nil
	}

	log := execution.Logger()
	result, err := e.evaluator.EvalExpression(execution, step.Condition, nil, nil, nil)
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
