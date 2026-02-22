package yaml

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/BDNK1/sflowg/runtime"
)

// StepExecutor dispatches step execution based on the step's Type field.
// Handles "assign" and "switch" as built-in types, delegates all others to the container's task registry.
type StepExecutor struct {
	evaluator runtime.ExpressionEvaluator
	l         *slog.Logger
}

func NewStepExecutor(evaluator runtime.ExpressionEvaluator, l *slog.Logger) *StepExecutor {
	return &StepExecutor{
		evaluator: evaluator,
		l:         l,
	}
}

func (e *StepExecutor) ExecuteStep(execution *runtime.Execution, step runtime.Step) (string, error) {
	switch step.Type {
	case "assign":
		return "", e.handleAssign(execution, step)
	case "switch":
		return e.handleSwitch(execution, step)
	case "return":
		return "", e.handleReturn(execution, step)
	default:
		return "", e.handleTask(execution, step)
	}
}

func (e *StepExecutor) handleAssign(execution *runtime.Execution, step runtime.Step) error {
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
func (e *StepExecutor) evaluateValue(execution *runtime.Execution, stepID string, path string, value any) (any, error) {
	switch v := value.(type) {
	case string:
		// Evaluate as expression - no fallback to literal
		// Use '"literal"' syntax for string literals in expressions
		result, err := e.evaluator.Eval(v, execution.Values())
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

func (e *StepExecutor) handleSwitch(execution *runtime.Execution, step runtime.Step) (string, error) {
	// Evaluate branches in the order their target steps appear in the flow,
	// so that catch-all/default branches defined last are evaluated last.
	orderedBranches := e.orderSwitchBranches(execution, step)

	for _, n := range orderedBranches {
		c := step.Args[n]
		condition, ok := c.(string)
		if !ok {
			return "", fmt.Errorf("switch condition must be a string expression, got %T", c)
		}

		result, err := e.evaluator.Eval(condition, execution.Values())
		if err != nil {
			e.l.ErrorContext(execution, fmt.Sprintf("Error evaluating switch condition for step %s, branch %s", step.ID, n),
				"condition", condition,
				"error", err,
				"values", execution.Values())
			return "", fmt.Errorf("error evaluating switch condition %s: %w", condition, err)
		}

		resultBool, ok := result.(bool)
		if !ok {
			return "", fmt.Errorf("condition %s evaluated to %T, expected boolean", condition, result)
		}

		if resultBool {
			e.l.InfoContext(execution, fmt.Sprintf("Resolving switch: %s is true", condition))
			return n, nil
		}
		e.l.InfoContext(execution, fmt.Sprintf("Resolving switch: %s is false", condition))
	}
	return "", nil
}

// orderSwitchBranches returns switch branch names ordered by the position of
// their target steps in the flow. Branches whose names don't match any step ID
// are appended at the end in alphabetical order.
func (e *StepExecutor) orderSwitchBranches(execution *runtime.Execution, step runtime.Step) []string {
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

func (e *StepExecutor) handleTask(execution *runtime.Execution, step runtime.Step) error {
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

// evaluateArgs evaluates all expressions in task args
func (e *StepExecutor) evaluateArgs(execution *runtime.Execution, step runtime.Step) (map[string]any, error) {
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

// executeTaskWithArgs executes a task with pre-evaluated args
func (e *StepExecutor) executeTaskWithArgs(execution *runtime.Execution, task runtime.Task, s runtime.Step, args map[string]any) {
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

	// Store results using SetNested to support nested access like step.result.row.id
	for k, v := range output {
		e.l.InfoContext(execution, fmt.Sprintf("DEBUG: Storing %s.result.%s = %v (type: %T)", s.ID, k, v, v))
		execution.Store.SetNested(fmt.Sprintf("%s.result.%s", s.ID, k), v)
	}
}

func getTaskNames(tasks map[string]runtime.Task) []string {
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	return names
}

func (e *StepExecutor) handleReturn(execution *runtime.Execution, step runtime.Step) error {
	evaluatedArgs, err := e.evaluateReturnArgs(execution.Flow.Return.Args, execution.Values())
	if err != nil {
		return fmt.Errorf("failed to evaluate return args: %w", err)
	}
	execution.ResponseDescriptor = &runtime.ResponseDescriptor{
		HandlerName: execution.Flow.Return.Type,
		Args:        evaluatedArgs,
	}
	return nil
}

// evaluateReturnArgs recursively evaluates all expressions in return arguments
func (e *StepExecutor) evaluateReturnArgs(args map[string]any, values map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, value := range args {
		evaluated, err := e.evaluateReturnArg(value, values)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate arg '%s': %w", key, err)
		}
		result[key] = evaluated
	}

	return result, nil
}

// evaluateReturnArg recursively evaluates a single argument value
func (e *StepExecutor) evaluateReturnArg(value any, values map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		// Try to evaluate as expression
		evaluated, err := e.evaluator.Eval(v, values)
		if err != nil {
			// If evaluation fails, return the original string
			// This allows literal strings in return args
			return v, nil
		}
		return evaluated, nil

	case map[string]any:
		// Recursively evaluate map values
		result := make(map[string]any)
		for k, val := range v {
			evaluated, err := e.evaluateReturnArg(val, values)
			if err != nil {
				return nil, err
			}
			result[k] = evaluated
		}
		return result, nil

	case []any:
		// Recursively evaluate array elements
		result := make([]any, len(v))
		for i, val := range v {
			evaluated, err := e.evaluateReturnArg(val, values)
			if err != nil {
				return nil, err
			}
			result[i] = evaluated
		}
		return result, nil

	default:
		// For other types (int, bool, etc.), return as-is
		return value, nil
	}
}
