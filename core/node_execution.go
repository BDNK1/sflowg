package runtime

import "fmt"

type nodeExecutionResult struct {
	Next string
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
	case FlowNodeForeach:
		if node.Foreach == nil {
			return nodeExecutionResult{}, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("foreach node %q has no block", node.ID), Step: node.ID}
		}
		return nodeExecutionResult{}, e.executeForeachBlock(execution, node, *node.Foreach, runCtx)
	default:
		return nodeExecutionResult{}, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("unsupported flow node kind %q", node.Kind), Step: node.ID}
	}
}
