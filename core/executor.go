package runtime

import (
	"fmt"
	"sync"
)

// Executor orchestrates flow node execution.
type Executor struct {
	evaluator    ExpressionEvaluator
	stepExecutor StepExecutor
	stepRunner   StepRunner
}

func NewExecutor(evaluator ExpressionEvaluator, stepExecutor StepExecutor, stepRunner StepRunner) *Executor {
	return &Executor{
		evaluator:    evaluator,
		stepExecutor: stepExecutor,
		stepRunner:   stepRunner,
	}
}

// ExecuteSteps runs all nodes in the flow for the given execution.
// The *Execution carries its own context (set by the HTTP handler with any
// flow-level timeout), so no separate ctx parameter is needed.
func (e *Executor) ExecuteSteps(execution *Execution) error {
	runCtx := &executionRunContext{
		flowFinalized: make(chan struct{}),
		detachedAsync: NewDetachedAsyncSupervisor(),
	}
	runCtx.detachedAsync.StartFailureLogger(runCtx.flowFinalized, execution.Logger(), execution.Metrics(), execFlowID(execution))
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
	detachedAsync    *DetachedAsyncSupervisor
	asyncWatcherOnce sync.Once
}

func (rc *executionRunContext) ensureAsyncWatcher(e *Executor, execution *Execution) {
	rc.asyncWatcherOnce.Do(func() {
		e.startAsyncCancellationWatcher(execution, rc.flowFinalized, rc.detachedAsync)
	})
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

func runtimeErrorForInvalidNext(stepID string, next string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: fmt.Sprintf("invalid __next target %q from %q", next, stepID),
		Step:    stepID,
	}
}
