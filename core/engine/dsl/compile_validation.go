package dsl

import (
	"context"
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

func validateFlowNodeIDs(nodes []runtime.FlowNode) error {
	seen := map[string]struct{}{}
	for _, node := range nodes {
		if node.ID == "" {
			return fmt.Errorf("top-level flow node is missing an id")
		}
		if _, exists := seen[node.ID]; exists {
			return fmt.Errorf("duplicate top-level flow node id %q", node.ID)
		}
		seen[node.ID] = struct{}{}
		if node.Kind == runtime.FlowNodeStep && hasReservedInternalPrefix(node.ID) {
			return fmt.Errorf("step ID %q uses reserved internal prefix", node.ID)
		}
	}
	return nil
}

func validateResultIDs(nodes []runtime.FlowNode, reserved map[string]struct{}) error {
	seen := map[string]struct{}{}
	for _, node := range nodes {
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step == nil {
				continue
			}
			if err := addResultID(seen, reserved, node.Step.ID); err != nil {
				return err
			}
		case runtime.FlowNodeParallel:
			if node.Parallel == nil {
				continue
			}
			for _, branch := range node.Parallel.Branches {
				if err := addResultID(seen, reserved, branch.ID); err != nil {
					return err
				}
			}
		case runtime.FlowNodeForeach:
			if node.Foreach == nil {
				continue
			}
			for _, collect := range node.Foreach.Collects {
				if err := addResultID(seen, reserved, collect.Alias); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addResultID(seen map[string]struct{}, reserved map[string]struct{}, id string) error {
	if id == "" {
		return fmt.Errorf("step ID is required")
	}
	if hasReservedInternalPrefix(id) {
		return fmt.Errorf("result ID %q uses reserved internal prefix", id)
	}
	if _, ok := reserved[id]; ok {
		return fmt.Errorf("result ID %q shadows framework key", id)
	}
	if _, exists := seen[id]; exists {
		return fmt.Errorf("duplicate result ID %q", id)
	}
	seen[id] = struct{}{}
	return nil
}

func reservedResultIDs(frameworkStoreKeys []string, frameworkKeys map[string]struct{}) map[string]struct{} {
	reserved := make(map[string]struct{}, len(frameworkStoreKeys)+len(frameworkKeys))
	for _, key := range frameworkStoreKeys {
		reserved[key] = struct{}{}
	}
	for key := range frameworkKeys {
		reserved[key] = struct{}{}
	}
	return reserved
}

func validateForeachIDs(cc *compileContext, block *runtime.ForeachBlock, parentStoreKeys []string, nodeIndex int) error {
	reserved := reservedResultIDs(parentStoreKeys, cc.frameworkKeys)
	if err := validateSimpleID("foreach loop variable", block.ItemVar); err != nil {
		return err
	}
	if hasReservedInternalPrefix(block.ItemVar) {
		return fmt.Errorf("foreach loop variable %q uses reserved internal prefix", block.ItemVar)
	}
	if _, ok := reserved[block.ItemVar]; ok {
		return fmt.Errorf("foreach loop variable %q shadows framework or parent key", block.ItemVar)
	}

	collectAliases := make(map[string]struct{}, len(block.Collects))
	for _, collect := range block.Collects {
		if err := validateSimpleID("collect alias", collect.Alias); err != nil {
			return err
		}
		if hasReservedInternalPrefix(collect.Alias) {
			return fmt.Errorf("collect alias %q uses reserved internal prefix", collect.Alias)
		}
		if _, ok := reserved[collect.Alias]; ok {
			return fmt.Errorf("collect alias %q shadows framework or parent key", collect.Alias)
		}
		if _, ok := collectAliases[collect.Alias]; ok {
			return fmt.Errorf("duplicate foreach collect alias %q", collect.Alias)
		}
		collectAliases[collect.Alias] = struct{}{}
	}

	bodyStepIDs := make(map[string]struct{}, len(block.Steps))
	for _, step := range block.Steps {
		if err := validateSimpleID("foreach body step ID", step.ID); err != nil {
			return err
		}
		if hasReservedInternalPrefix(step.ID) {
			return fmt.Errorf("step ID %q uses reserved internal prefix", step.ID)
		}
		if _, ok := bodyStepIDs[step.ID]; ok {
			return fmt.Errorf("duplicate foreach body step %q", step.ID)
		}
		bodyStepIDs[step.ID] = struct{}{}
		if step.ID == block.ItemVar {
			return fmt.Errorf("foreach body step ID %q shadows loop variable", step.ID)
		}
		if _, ok := reserved[step.ID]; ok {
			return fmt.Errorf("foreach body step ID %q shadows framework or parent key", step.ID)
		}
		if _, ok := collectAliases[step.ID]; ok {
			return fmt.Errorf("foreach body step ID %q collides with collect alias", step.ID)
		}
		if step.Async {
			if asyncIndex, ok := cc.asyncStepIndexes[step.ID]; ok && asyncIndex < nodeIndex {
				return fmt.Errorf("foreach async step ID %q shadows prior parent async step", step.ID)
			}
		}
	}
	return nil
}

func validateSimpleID(label string, id string) error {
	if id == "" {
		return fmt.Errorf("%s is required", label)
	}
	for i := 0; i < len(id); i++ {
		if !isWordChar(id[i]) {
			return fmt.Errorf("%s %q must be a simple identifier", label, id)
		}
	}
	return nil
}

func validateStepRefsVisible(cc *compileContext, step *runtime.Step, index int, visibleKeys []string) error {
	return validateStepRefsVisibleWithKeys(cc, step, index, visibleKeys, cc.allReferenceKeys)
}

func validateForeachStepRefsVisible(cc *compileContext, step *runtime.Step, index int, visibleKeys []string, allForeachKeys []string) error {
	return validateStepRefsVisibleWithKeys(cc, step, index, visibleKeys, allForeachKeys)
}

func validateStepRefsVisibleWithKeys(cc *compileContext, step *runtime.Step, index int, visibleKeys []string, allStoreKeys []string) error {
	if step == nil {
		return nil
	}
	if err := validateRefsVisibleWithKeys(cc, step.ID, index, "body", step.Body, visibleKeys, allStoreKeys); err != nil {
		return err
	}
	if err := validateRefsVisibleWithKeys(cc, step.ID, index, "fallback", step.FallbackBody, visibleKeys, allStoreKeys); err != nil {
		return err
	}
	if err := validateRefsVisibleWithKeys(cc, step.ID, index, "condition", step.Condition, visibleKeys, allStoreKeys); err != nil {
		return err
	}
	if step.Retry != nil {
		if err := validateRefsVisibleWithKeys(cc, step.ID, index, "retry", step.Retry.When, visibleKeys, allStoreKeys); err != nil {
			return err
		}
	}
	compensateVisibleKeys := append(append([]string{}, visibleKeys...), step.ID)
	if err := validateRefsVisibleWithKeys(cc, step.ID, index, "compensate", step.CompensateBody, compensateVisibleKeys, allStoreKeys); err != nil {
		return err
	}
	return nil
}

func validateRefsVisible(cc *compileContext, consumer string, index int, surface string, source string, visibleKeys []string) error {
	return validateRefsVisibleWithKeys(cc, consumer, index, surface, source, visibleKeys, cc.allReferenceKeys)
}

func validateRefsVisibleWithKeys(cc *compileContext, consumer string, index int, surface string, source string, visibleKeys []string, allStoreKeys []string) error {
	refs, err := cc.extractStoreKeys(source, allStoreKeys)
	if err != nil {
		return fmt.Errorf("compile %s for %s: %w", surface, consumer, err)
	}
	visible := stringSet(visibleKeys)
	for _, ref := range refs {
		if _, ok := visible[ref]; !ok {
			if asyncIndex, ok := cc.asyncStepIndexes[ref]; ok && asyncIndex >= index {
				return fmt.Errorf("step %q %s references async step %q before it is spawned", consumer, surface, ref)
			}
			return fmt.Errorf("%s %q %s references %q outside lexical scope", surface, consumer, surface, ref)
		}
	}
	return nil
}

func rejectResponseCallsInStep(step *runtime.Step) error {
	if step == nil {
		return nil
	}
	if err := rejectResponseCallsInSurface(step.ID, "body", step.Body); err != nil {
		return err
	}
	if err := rejectResponseCallsInSurface(step.ID, "fallback", step.FallbackBody); err != nil {
		return err
	}
	if err := rejectResponseCallsInSurface(step.ID, "condition", step.Condition); err != nil {
		return err
	}
	if step.Retry != nil {
		if err := rejectResponseCallsInSurface(step.ID, "retry", step.Retry.When); err != nil {
			return err
		}
	}
	return nil
}

func hasReservedInternalPrefix(id string) bool {
	return strings.HasPrefix(id, runtime.InternalParallelNodePrefix) ||
		strings.HasPrefix(id, runtime.InternalForeachNodePrefix)
}

func validateNoPeerRefs(branchID string, surface string, storeKeys []string, peerIDs map[string]struct{}) error {
	if len(peerIDs) == 0 {
		return nil
	}
	for _, key := range storeKeys {
		if key == branchID {
			continue
		}
		if _, ok := peerIDs[key]; ok {
			return fmt.Errorf("parallel branch %q %s references peer branch %q", branchID, surface, key)
		}
	}
	return nil
}

func validateNoUserNext(stepID string, surface string, source string, allowed bool) error {
	if allowed || !strings.Contains(source, "__next") {
		return nil
	}
	hasNext, err := returnedMapHasLiteralKey(source, "__next")
	if err != nil {
		return nil
	}
	if !hasNext {
		return nil
	}
	return fmt.Errorf("step %q %s cannot return __next", stepID, surface)
}

func returnedMapHasLiteralKey(source string, key string) (bool, error) {
	program, err := risorparser.Parse(context.Background(), source, nil)
	if err != nil {
		return false, err
	}
	if len(program.Stmts) == 0 {
		return false, nil
	}
	expr := returnedExpr(program.Stmts[len(program.Stmts)-1])
	m, ok := expr.(*ast.Map)
	if !ok {
		return false, nil
	}
	for _, item := range m.Items {
		if item.Key == nil {
			continue
		}
		itemKey, ok := literalMapKey(item.Key)
		if ok && itemKey == key {
			return true, nil
		}
	}
	return false, nil
}

func returnedExpr(node ast.Node) ast.Expr {
	switch n := node.(type) {
	case ast.Expr:
		return n
	case *ast.Return:
		return n.Value
	default:
		return nil
	}
}
