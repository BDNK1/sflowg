package dsl

import (
	"sort"

	"github.com/BDNK1/sflowg/core"
)

func collectFrameworkStoreKeys(flow *runtime.Flow) []string {
	keys := []string{"request", "input", "properties", "error", "compensation"}
	if flow.Entrypoint.Type == "kafka" {
		keys = append(keys, "message")
	}
	if flow.Entrypoint.Type == "cron" {
		keys = append(keys, "trigger")
	}
	return keys
}

func visibleKeysAfterAllNodes(nodes []runtime.FlowNode, frameworkStoreKeys []string) []string {
	return visibleKeysBeforeNode(nodes, len(nodes), frameworkStoreKeys)
}

func visibleKeysBeforeNode(nodes []runtime.FlowNode, index int, frameworkStoreKeys []string) []string {
	keys := append([]string{}, frameworkStoreKeys...)
	seen := make(map[string]struct{}, len(keys)+len(nodes))
	for _, key := range keys {
		seen[key] = struct{}{}
	}
	for i := 0; i < index && i < len(nodes); i++ {
		for _, key := range resultKeysProducedByNode(nodes[i]) {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func resultKeysProducedByNode(node runtime.FlowNode) []string {
	switch node.Kind {
	case runtime.FlowNodeStep:
		if node.Step == nil {
			return nil
		}
		return []string{node.Step.ID}
	case runtime.FlowNodeParallel:
		if node.Parallel == nil {
			return nil
		}
		keys := make([]string, 0, len(node.Parallel.Branches))
		for _, branch := range node.Parallel.Branches {
			keys = append(keys, branch.ID)
		}
		return keys
	case runtime.FlowNodeForeach:
		if node.Foreach == nil {
			return nil
		}
		keys := make([]string, 0, len(node.Foreach.Collects))
		for _, collect := range node.Foreach.Collects {
			keys = append(keys, collect.Alias)
		}
		return keys
	default:
		return nil
	}
}

func foreachBodyStepKeys(nodes []runtime.FlowNode) []string {
	var keys []string
	for _, node := range nodes {
		if node.Kind != runtime.FlowNodeForeach || node.Foreach == nil {
			continue
		}
		for _, step := range node.Foreach.Steps {
			keys = append(keys, step.ID)
		}
	}
	return keys
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func filterParentStoreKeys(keys []string, itemVar string, localStepIDs map[string]struct{}, collectAliases map[string]struct{}) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == itemVar {
			continue
		}
		if _, ok := localStepIDs[key]; ok {
			continue
		}
		if _, ok := collectAliases[key]; ok {
			continue
		}
		out = append(out, key)
	}
	return out
}
