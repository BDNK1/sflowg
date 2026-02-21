package runtime

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Executor orchestrates flow step execution.
// It handles the step loop, condition evaluation, and retry logic,
// delegating actual step execution to a StepExecutor.
type Executor struct {
	l            *slog.Logger
	evaluator    ExpressionEvaluator
	stepExecutor StepExecutor
}

func NewExecutor(l *slog.Logger, evaluator ExpressionEvaluator, stepExecutor StepExecutor) *Executor {
	return &Executor{
		l:            l,
		evaluator:    evaluator,
		stepExecutor: stepExecutor,
	}
}

func (e *Executor) ExecuteSteps(execution *Execution) error {
	nextStep := ""

	for _, s := range execution.Flow.Steps {
		if nextStep != "" {
			if s.ID != nextStep {
				e.l.InfoContext(execution, fmt.Sprintf("Skipping step: %s", s.ID))
				continue
			}
			nextStep = ""
			e.l.InfoContext(execution, fmt.Sprintf("Resuming flow at step: %s", s.ID))
		}

		if err := e.evaluateCondition(execution, s); err != nil {
			e.l.InfoContext(execution, fmt.Sprintf("Skipping step: %s", s.ID))
			continue
		}

		next, err := e.stepExecutor.ExecuteStep(execution, s)
		if err != nil {
			return fmt.Errorf("error executing step %s: %w", s.ID, err)
		}

		if execution.ResponseDescriptor != nil {
			e.l.InfoContext(execution, fmt.Sprintf("Response produced at step: %s", s.ID))
			break
		}

		if next != "" {
			nextStep = next
		}

		if err := e.handleRetry(execution, s); err != nil {
			return fmt.Errorf("error retrying step %s: %w", s.ID, err)
		}
	}

	return nil
}

func (e *Executor) evaluateCondition(execution *Execution, step Step) error {
	if step.Condition == "" {
		return nil
	}

	result, err := e.evaluator.Eval(step.Condition, execution.Values())
	if err != nil {
		e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating condition for step %s", step.ID),
			"condition", step.Condition,
			"error", err,
			"values", execution.Values())
		return fmt.Errorf("error evaluating condition %s: %w", step.Condition, err)
	}

	resultBool, ok := result.(bool)
	if !ok {
		return fmt.Errorf("condition %s evaluated to %T, expected boolean", step.Condition, result)
	}
	if !resultBool {
		return fmt.Errorf("condition not met: %s", step.Condition)
	}
	e.l.InfoContext(execution, fmt.Sprintf("Condition met: %s", step.Condition))
	return nil
}

func (e *Executor) handleRetry(execution *Execution, step Step) error {
	if step.Retry == nil {
		return nil
	}

	for i := 0; i < step.Retry.MaxRetries; i++ {
		condition, err := e.evaluator.Eval(step.Retry.Condition, execution.Values())
		e.l.InfoContext(execution, fmt.Sprintf("[%s/%s] Retrying step: %s, condition: %v", strconv.Itoa(i+1), strconv.Itoa(step.Retry.MaxRetries), step.ID, condition))

		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating retry condition for step %s", step.ID),
				"condition", step.Retry.Condition,
				"error", err,
				"values", execution.Values())
			return fmt.Errorf("error evaluating retry condition %s: %w", step.Retry.Condition, err)
		}

		conditionBool, ok := condition.(bool)
		if !ok {
			return fmt.Errorf("retry condition evaluated to %T, expected boolean", condition)
		}
		if !conditionBool {
			break
		}

		delay := time.Duration(step.Retry.Delay) * time.Millisecond
		if step.Retry.Backoff {
			delay = time.Duration(i*step.Retry.Delay) * time.Millisecond
		}
		time.Sleep(delay)

		// Re-execute the step (StepExecutor handles re-evaluating args)
		if _, err := e.stepExecutor.ExecuteStep(execution, step); err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error re-executing step %s during retry", step.ID),
				"error", err)
		}
	}
	return nil
}
