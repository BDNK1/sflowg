package runtime

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"
)

type Executor struct {
	l *slog.Logger
}

func NewExecutor(l *slog.Logger) *Executor {
	return &Executor{l: l}
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

		if err := e.executeStepType(execution, s, &nextStep); err != nil {
			return fmt.Errorf("error executing step %s: %w", s.ID, err)
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

	result, err := Eval(step.Condition, execution.Values)
	if err != nil {
		e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating condition for step %s", step.ID),
			"condition", step.Condition,
			"error", err,
			"values", execution.Values)
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

func (e *Executor) executeStepType(execution *Execution, step Step, nextStep *string) error {
	switch step.Type {
	case "assign":
		return e.handleAssign(execution, step)
	case "switch":
		return e.handleSwitch(execution, step, nextStep)
	default:
		return e.handleTask(execution, step)
	}
}

func (e *Executor) handleAssign(execution *Execution, step Step) error {
	for k, v := range step.Args {
		evaluated, err := e.evaluateValue(execution, step.ID, k, v)
		if err != nil {
			return err
		}
		// Store with step ID prefix so it can be accessed as {stepID}.{key}
		execution.AddValue(fmt.Sprintf("%s.%s", step.ID, k), evaluated)
	}
	return nil
}

// evaluateValue recursively evaluates expressions in nested structures
func (e *Executor) evaluateValue(execution *Execution, stepID string, path string, value any) (any, error) {
	switch v := value.(type) {
	case string:
		// Evaluate as expression - no fallback to literal
		// Use '"literal"' syntax for string literals in expressions
		result, err := Eval(v, execution.Values)
		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating expression for step %s, path %s", stepID, path),
				"expression", v,
				"error", err)
			return nil, fmt.Errorf("error evaluating expression '%s': %w", v, err)
		}
		return result, nil
	case map[string]any:
		// Recursively evaluate all values in the map
		evaluated := make(map[string]any)
		for key, val := range v {
			nestedPath := fmt.Sprintf("%s.%s", path, key)
			evaluatedVal, err := e.evaluateValue(execution, stepID, nestedPath, val)
			if err != nil {
				return nil, err
			}
			evaluated[key] = evaluatedVal
		}
		return evaluated, nil
	case []any:
		// Recursively evaluate all elements in the array
		evaluated := make([]any, len(v))
		for i, val := range v {
			nestedPath := fmt.Sprintf("%s[%d]", path, i)
			evaluatedVal, err := e.evaluateValue(execution, stepID, nestedPath, val)
			if err != nil {
				return nil, err
			}
			evaluated[i] = evaluatedVal
		}
		return evaluated, nil
	default:
		// Return literal values as-is (int, bool, float64, etc.)
		return value, nil
	}
}

func (e *Executor) handleSwitch(execution *Execution, step Step, nextStep *string) error {
	// Evaluate branches in the order their target steps appear in the flow,
	// so that catch-all/default branches defined last are evaluated last.
	orderedBranches := e.orderSwitchBranches(execution, step)

	for _, n := range orderedBranches {
		c := step.Args[n]
		condition, ok := c.(string)
		if !ok {
			return fmt.Errorf("switch condition must be a string expression, got %T", c)
		}

		result, err := Eval(condition, execution.Values)
		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating switch condition for step %s, branch %s", step.ID, n),
				"condition", condition,
				"error", err,
				"values", execution.Values)
			return fmt.Errorf("error evaluating switch condition %s: %w", condition, err)
		}

		resultBool, ok := result.(bool)
		if !ok {
			return fmt.Errorf("condition %s evaluated to %T, expected boolean", condition, result)
		}

		if resultBool {
			e.l.InfoContext(execution, fmt.Sprintf("Resolving switch: %s is true", condition))
			*nextStep = n
			return nil
		}
		e.l.InfoContext(execution, fmt.Sprintf("Resolving switch: %s is false", condition))
	}
	return nil
}

// orderSwitchBranches returns switch branch names ordered by the position of
// their target steps in the flow. Branches whose names don't match any step ID
// are appended at the end in alphabetical order.
func (e *Executor) orderSwitchBranches(execution *Execution, step Step) []string {
	stepOrder := make(map[string]int, len(execution.Flow.Steps))
	for i, s := range execution.Flow.Steps {
		stepOrder[s.ID] = i
	}

	matched := make([]string, 0, len(step.Args))
	unmatched := make([]string, 0)
	for name := range step.Args {
		if _, ok := stepOrder[name]; ok {
			matched = append(matched, name)
		} else {
			unmatched = append(unmatched, name)
		}
	}

	sort.Strings(unmatched)
	sort.Slice(matched, func(i, j int) bool {
		return stepOrder[matched[i]] < stepOrder[matched[j]]
	})

	return append(matched, unmatched...)
}

func (e *Executor) handleTask(execution *Execution, step Step) error {
	e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Looking for task type: %s", step.Type))

	task, ok := execution.Container.Tasks[step.Type]
	if !ok {
		e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Available tasks: %v", getTaskNames(execution.Container.Tasks)))
		return fmt.Errorf("task type: %s not found", step.Type)
	}

	e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Found task, type=%T", task))

	// Evaluate all args before passing to task
	evaluatedArgs, err := e.evaluateArgs(execution, step)
	if err != nil {
		return fmt.Errorf("failed to evaluate args for task %s: %w", step.Type, err)
	}

	e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Calling task.Execute with args: %+v", evaluatedArgs))

	e.executeTaskWithArgs(execution, task, step, evaluatedArgs)
	e.l.InfoContext(execution, fmt.Sprintf("Executed task: %s", step.Type))
	return nil
}

func getTaskNames(tasks map[string]Task) []string {
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	return names
}

// evaluateArgs evaluates all expressions in task args
func (e *Executor) evaluateArgs(execution *Execution, step Step) (map[string]any, error) {
	evaluated := make(map[string]any)
	for k, v := range step.Args {
		result, err := e.evaluateValue(execution, step.ID, k, v)
		if err != nil {
			return nil, err
		}
		evaluated[k] = result
	}
	return evaluated, nil
}

func (e *Executor) handleRetry(execution *Execution, step Step) error {
	if step.Retry == nil {
		return nil
	}

	task, ok := execution.Container.Tasks[step.Type]
	if !ok {
		return fmt.Errorf("Task type: %s not found", step.Type)
	}

	for i := 0; i < step.Retry.MaxRetries; i++ {
		condition, err := Eval(step.Retry.Condition, execution.Values)
		e.l.InfoContext(execution, fmt.Sprintf("[%s/%s] Retrying step: %s, condition: %v", strconv.Itoa(i+1), strconv.Itoa(step.Retry.MaxRetries), step.ID, condition))

		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating retry condition for step %s", step.ID),
				"condition", step.Retry.Condition,
				"error", err,
				"values", execution.Values)
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

		// Re-evaluate args for retry (values may have changed)
		evaluatedArgs, err := e.evaluateArgs(execution, step)
		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating args for retry step %s", step.ID),
				"error", err)
			continue
		}
		e.executeTaskWithArgs(execution, task, step, evaluatedArgs)
	}
	return nil
}

// executeTaskWithArgs executes a task with pre-evaluated args
func (e *Executor) executeTaskWithArgs(execution *Execution, task Task, s Step, args map[string]any) {
	e.l.InfoContext(execution, fmt.Sprintf("DEBUG: executeTaskWithArgs START for step %s", s.ID))

	output, err := task.Execute(execution, args)

	e.l.InfoContext(execution, fmt.Sprintf("DEBUG: executeTaskWithArgs DONE - output=%+v, err=%v", output, err))

	if err != nil {
		e.l.ErrorContext(execution, fmt.Sprintf("Task execution failed: %s", s.ID),
			"task_type", s.Type,
			"error", err.Error(),
			"args", args)
		execution.AddValue(fmt.Sprintf("%s.error", s.ID), err.Error())
	}

	// Store results recursively to support nested access like step.result.row.id
	for k, v := range output {
		e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Storing %s.result.%s = %v (type: %T)", s.ID, k, v, v))
		storeResultWithIntermediates(execution, fmt.Sprintf("%s.result.%s", s.ID, k), v)
	}
}

// storeResultWithIntermediates recursively stores task output values at every level.
// This enables accessing nested results like step.result.row.id
func storeResultWithIntermediates(e *Execution, prefix string, value any) {
	e.AddValue(prefix, value)

	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			storeResultWithIntermediates(e, prefix+"."+k, v)
		}
	}

	if arr, ok := value.([]any); ok {
		for i, v := range arr {
			storeResultWithIntermediates(e, fmt.Sprintf("%s.%d", prefix, i), v)
		}
	}
}
