package dsl

import (
	"fmt"
	"sort"

	"github.com/BDNK1/sflowg/core"
)

func collectAsyncStepIndexes(flow *runtime.Flow) map[string]int {
	indexes := make(map[string]int)
	for i, node := range runtime.NormalizeFlowNodes(flow) {
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step != nil && node.Step.Async {
				indexes[node.Step.ID] = i
			}
		case runtime.FlowNodeParallel:
			if node.Parallel == nil {
				continue
			}
			for _, branch := range node.Parallel.Branches {
				if branch.Async {
					indexes[branch.ID] = i
				}
			}
		case runtime.FlowNodeForeach:
		}
	}
	return indexes
}

func validateNoForwardAsyncRefs(consumer string, consumerIndex int, surface string, storeKeys []string, asyncIndexes map[string]int) error {
	for _, key := range storeKeys {
		asyncIndex, ok := asyncIndexes[key]
		if !ok {
			continue
		}
		if asyncIndex >= consumerIndex {
			return fmt.Errorf("step %q %s references async step %q before it is spawned", consumer, surface, key)
		}
	}
	return nil
}

func validateNoForwardParentAsyncRefs(
	consumer string,
	consumerIndex int,
	surface string,
	storeKeys []string,
	asyncIndexes map[string]int,
	localNames map[string]struct{},
) error {
	filtered := make([]string, 0, len(storeKeys))
	for _, key := range storeKeys {
		if _, ok := localNames[key]; ok {
			continue
		}
		filtered = append(filtered, key)
	}
	return validateNoForwardAsyncRefs(consumer, consumerIndex, surface, filtered, asyncIndexes)
}

func intersectAsyncDeps(asyncIndexes map[string]int, groups ...[]string) []string {
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, key := range group {
			if _, ok := asyncIndexes[key]; ok {
				seen[key] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	deps := make([]string, 0, len(seen))
	for key := range seen {
		deps = append(deps, key)
	}
	sort.Strings(deps)
	return deps
}
