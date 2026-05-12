package dsl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BDNK1/sflowg/core"
)

func (c *Compiler) compileForeach(
	cc *compileContext,
	nodeID string,
	block *runtime.ForeachBlock,
	index int,
	parentStoreKeys []string,
) error {
	if !strings.HasPrefix(nodeID, runtime.InternalForeachNodePrefix) {
		return fmt.Errorf("foreach node %q must use reserved internal prefix %q", nodeID, runtime.InternalForeachNodePrefix)
	}
	if err := validateForeachIDs(cc, block, parentStoreKeys, index); err != nil {
		return err
	}
	if err := validateRefsVisible(cc, nodeID, index, "source", block.Expr, parentStoreKeys); err != nil {
		return err
	}
	storeKeys, compiled, err := c.compileExpression(cc, block.Expr, parentStoreKeys)
	if err != nil {
		return fmt.Errorf("compile foreach %s source: %w", nodeID, err)
	}
	block.ExprStoreKeys = storeKeys
	block.ExprProgram = compiled
	if err := validateNoForwardParentAsyncRefs(nodeID, index, "source", block.ExprStoreKeys, cc.asyncStepIndexes, nil); err != nil {
		return err
	}

	localStepIDs := make(map[string]struct{}, len(block.Steps))
	localNames := make(map[string]struct{}, len(block.Steps)+1)
	if block.ItemVar != "" {
		localNames[block.ItemVar] = struct{}{}
	}
	for _, step := range block.Steps {
		localStepIDs[step.ID] = struct{}{}
		localNames[step.ID] = struct{}{}
	}

	localAsyncIndexes := map[string]int{}
	for i, step := range block.Steps {
		if step.Async {
			localAsyncIndexes[step.ID] = i
		}
	}
	localAsyncIDs := make([]string, 0, len(localAsyncIndexes))
	for id := range localAsyncIndexes {
		localAsyncIDs = append(localAsyncIDs, id)
	}
	sort.Strings(localAsyncIDs)
	allForeachKeys := append(append([]string{}, cc.allReferenceKeys...), block.ItemVar)
	for _, step := range block.Steps {
		allForeachKeys = append(allForeachKeys, step.ID)
	}
	for _, collect := range block.Collects {
		allForeachKeys = append(allForeachKeys, collect.Alias)
	}
	allForeachKeys = dedupeStrings(allForeachKeys)

	for i := range block.Steps {
		if block.Steps[i].CompensateBody != "" {
			return fmt.Errorf("foreach step %q cannot have compensate block", block.Steps[i].ID)
		}
		stepVisible := append([]string{}, parentStoreKeys...)
		stepVisible = append(stepVisible, block.ItemVar)
		stepVisible = append(stepVisible, localAsyncIDs...)
		for j := 0; j < i; j++ {
			stepVisible = append(stepVisible, block.Steps[j].ID)
		}
		stepVisible = dedupeStrings(stepVisible)
		if err := rejectResponseCallsInStep(&block.Steps[i]); err != nil {
			return err
		}
		if err := validateForeachStepRefsVisible(cc, &block.Steps[i], index, stepVisible, allForeachKeys); err != nil {
			return err
		}
		if err := c.compileLocalStep(cc, &block.Steps[i], i, stepVisible, localAsyncIndexes); err != nil {
			return err
		}
		if err := validateNoForwardParentAsyncRefs(block.Steps[i].ID, index, "body", block.Steps[i].StoreKeys, cc.asyncStepIndexes, localNames); err != nil {
			return err
		}
		if err := validateNoForwardParentAsyncRefs(block.Steps[i].ID, index, "fallback", block.Steps[i].FallbackStoreKeys, cc.asyncStepIndexes, localNames); err != nil {
			return err
		}
		conditionStoreKeys, err := cc.extractStoreKeys(block.Steps[i].Condition, stepVisible)
		if err != nil {
			return fmt.Errorf("compile condition for step %s: %w", block.Steps[i].ID, err)
		}
		if err := validateNoForwardParentAsyncRefs(block.Steps[i].ID, index, "condition", conditionStoreKeys, cc.asyncStepIndexes, localNames); err != nil {
			return err
		}
		var retryStoreKeys []string
		if block.Steps[i].Retry != nil {
			retryStoreKeys, err = cc.extractStoreKeys(block.Steps[i].Retry.When, stepVisible)
			if err != nil {
				return fmt.Errorf("compile retry for step %s: %w", block.Steps[i].ID, err)
			}
			if err := validateNoForwardParentAsyncRefs(block.Steps[i].ID, index, "retry", retryStoreKeys, cc.asyncStepIndexes, localNames); err != nil {
				return err
			}
		}
		parentAsyncDeps := intersectAsyncDeps(cc.asyncStepIndexes, block.Steps[i].StoreKeys, block.Steps[i].FallbackStoreKeys, conditionStoreKeys, retryStoreKeys)
		block.Steps[i].AsyncDeps = dedupeStrings(append(block.Steps[i].AsyncDeps, parentAsyncDeps...))
	}

	parentKeys := append([]string{}, block.ExprStoreKeys...)
	for _, step := range block.Steps {
		parentKeys = append(parentKeys, step.StoreKeys...)
		parentKeys = append(parentKeys, step.FallbackStoreKeys...)
		if conditionKeys, err := cc.extractStoreKeys(step.Condition, allForeachKeys); err == nil {
			parentKeys = append(parentKeys, conditionKeys...)
		}
		if step.Retry != nil {
			if retryKeys, err := cc.extractStoreKeys(step.Retry.When, allForeachKeys); err == nil {
				parentKeys = append(parentKeys, retryKeys...)
			}
		}
	}
	collectAliases := make(map[string]struct{}, len(block.Collects))
	collectVisible := append([]string{}, parentStoreKeys...)
	collectVisible = append(collectVisible, block.ItemVar)
	for _, step := range block.Steps {
		collectVisible = append(collectVisible, step.ID)
	}
	collectVisible = dedupeStrings(collectVisible)
	for i := range block.Collects {
		collect := &block.Collects[i]
		collectAliases[collect.Alias] = struct{}{}
		if err := validateRefsVisibleWithKeys(cc, collect.Alias, index, "collect", collect.Expr, collectVisible, allForeachKeys); err != nil {
			return err
		}
		storeKeys, compiled, err := c.compileExpression(cc, collect.Expr, collectVisible)
		if err != nil {
			return fmt.Errorf("compile foreach %s collect %s: %w", nodeID, collect.Alias, err)
		}
		if err := validateNoForwardAsyncRefs(collect.Alias, len(block.Steps), "collect", storeKeys, localAsyncIndexes); err != nil {
			return err
		}
		if err := validateNoForwardParentAsyncRefs(collect.Alias, index, "collect", storeKeys, cc.asyncStepIndexes, localNames); err != nil {
			return err
		}
		collect.StoreKeys = storeKeys
		collect.ExprProgram = compiled
		parentKeys = append(parentKeys, storeKeys...)
	}

	block.ParentStoreKeys = filterParentStoreKeys(dedupeStrings(parentKeys), block.ItemVar, localStepIDs, collectAliases)
	return nil
}
