package dsl

import (
	"context"
	"fmt"

	"github.com/BDNK1/sflowg/core"
)

type Compiler struct {
	interpreter *Interpreter
}

func NewCompiler() *Compiler {
	return &Compiler{interpreter: &Interpreter{}}
}

func (c *Compiler) CompileFlow(ctx context.Context, flow *runtime.Flow, container *runtime.Container) error {
	cc := newCompileContext(ctx, flow, container)

	if err := validateFlowNodeIDs(cc.nodes); err != nil {
		return err
	}
	if err := validateResultIDs(cc.nodes, cc.reservedResultIDs()); err != nil {
		return err
	}

	for i := range cc.nodes {
		node := &cc.nodes[i]
		visibleKeys := cc.visibleKeysBeforeNode(i)
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step == nil {
				return fmt.Errorf("step node %q has no step", node.ID)
			}
			if err := validateStepRefsVisible(cc, node.Step, i, visibleKeys); err != nil {
				return err
			}
			if err := c.compileStep(cc, node.Step, i, visibleKeys, false, nil); err != nil {
				return err
			}
		case runtime.FlowNodeParallel:
			if node.Parallel == nil {
				return fmt.Errorf("parallel node %q has no block", node.ID)
			}
			peerIDs := map[string]struct{}{}
			for _, branch := range node.Parallel.Branches {
				peerIDs[branch.ID] = struct{}{}
			}
			for j := range node.Parallel.Branches {
				branch := &node.Parallel.Branches[j]
				if err := validateStepRefsVisible(cc, branch, i, visibleKeys); err != nil {
					return err
				}
				if err := c.compileStep(cc, branch, i, visibleKeys, true, peerIDs); err != nil {
					return err
				}
			}
		case runtime.FlowNodeForeach:
			if node.Foreach == nil {
				return fmt.Errorf("foreach node %q has no block", node.ID)
			}
			if err := c.compileForeach(cc, node.ID, node.Foreach, i, visibleKeys); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported flow node kind %q", node.Kind)
		}
	}

	if len(flow.Nodes) > 0 {
		flow.Nodes = cc.nodes
	}
	syncCompiledSteps(flow, cc.nodes)

	if err := validateRefsVisible(cc, "on_error", len(cc.nodes), "body", flow.OnErrorBody, cc.allStoreKeys); err != nil {
		return err
	}
	_, onErrorCompiled, err := c.compileBody(cc, flow.OnErrorBody, cc.allStoreKeys)
	if err != nil {
		return fmt.Errorf("compile on_error: %w", err)
	}
	flow.OnErrorCompiled = onErrorCompiled

	if err := validateRefsVisible(cc, "__return", len(cc.nodes), "return", flow.Return.Body, cc.allStoreKeys); err != nil {
		return err
	}
	returnStoreKeys, err := cc.extractStoreKeys(flow.Return.Body, cc.allStoreKeys)
	if err != nil {
		return fmt.Errorf("compile return: %w", err)
	}
	if err := validateNoForwardAsyncRefs("__return", len(cc.nodes), "return", returnStoreKeys, cc.asyncStepIndexes); err != nil {
		return err
	}

	return nil
}

func syncCompiledSteps(flow *runtime.Flow, nodes []runtime.FlowNode) {
	if len(flow.Steps) == 0 {
		return
	}
	compiledByID := map[string]runtime.Step{}
	for _, node := range nodes {
		if node.Kind == runtime.FlowNodeStep && node.Step != nil {
			compiledByID[node.Step.ID] = *node.Step
		}
	}
	for i := range flow.Steps {
		if step, ok := compiledByID[flow.Steps[i].ID]; ok {
			flow.Steps[i] = step
		}
	}
}
