package dsl

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/ast"
	risorparser "github.com/deepnoodle-ai/risor/v2/pkg/parser"
)

type Compiler struct {
	interpreter *Interpreter
}

func NewCompiler() *Compiler {
	return &Compiler{interpreter: &Interpreter{}}
}

func (c *Compiler) CompileFlow(ctx context.Context, flow *runtime.Flow, container *runtime.Container) error {
	frameworkKeys, frameworkEnv := collectFrameworkInfo(container)
	contract := responseContractForFlow(flow)
	frameworkEnv["response"] = buildResponseTemplateModule(contract)
	knownStoreKeys := collectKnownStoreKeys(flow)
	asyncStepIndexes := collectAsyncStepIndexes(flow)
	nodes := runtime.NormalizeFlowNodes(flow)

	if err := validateFlowNodeIDs(nodes); err != nil {
		return err
	}
	if err := validateResultIDs(nodes); err != nil {
		return err
	}

	for i := range nodes {
		node := &nodes[i]
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step == nil {
				return fmt.Errorf("step node %q has no step", node.ID)
			}
			if err := c.compileStep(ctx, node.Step, i, knownStoreKeys, frameworkKeys, frameworkEnv, contract, asyncStepIndexes, false, nil); err != nil {
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
				if err := c.compileStep(ctx, branch, i, knownStoreKeys, frameworkKeys, frameworkEnv, contract, asyncStepIndexes, true, peerIDs); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported flow node kind %q", node.Kind)
		}
	}

	if len(flow.Nodes) > 0 {
		flow.Nodes = nodes
	}
	syncCompiledSteps(flow, nodes)

	_, onErrorCompiled, err := c.compileBody(ctx, flow.OnErrorBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
	if err != nil {
		return fmt.Errorf("compile on_error: %w", err)
	}
	flow.OnErrorCompiled = onErrorCompiled

	returnStoreKeys, err := extractStoreKeys(flow.Return.Body, knownStoreKeys, frameworkKeys)
	if err != nil {
		return fmt.Errorf("compile return: %w", err)
	}
	if err := validateNoForwardAsyncRefs("__return", len(nodes), "return", returnStoreKeys, asyncStepIndexes); err != nil {
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

func (c *Compiler) compileStep(
	ctx context.Context,
	step *runtime.Step,
	index int,
	knownStoreKeys []string,
	frameworkKeys map[string]struct{},
	frameworkEnv map[string]any,
	contract runtime.ResponseContract,
	asyncStepIndexes map[string]int,
	inParallel bool,
	peerIDs map[string]struct{},
) error {
	if strings.HasPrefix(step.ID, runtime.InternalParallelNodePrefix) {
		return fmt.Errorf("step ID %q uses reserved internal prefix %q", step.ID, runtime.InternalParallelNodePrefix)
	}
	if inParallel && step.CompensateBody != "" {
		return fmt.Errorf("parallel branch %q cannot have compensate block", step.ID)
	}
	if err := validateNoUserNext(step.ID, "body", step.Body, step.AllowsNext); err != nil {
		return err
	}
	if err := validateNoUserNext(step.ID, "fallback", step.FallbackBody, false); err != nil {
		return err
	}
	if inParallel {
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
	}

	storeKeys, compiled, err := c.compileBody(ctx, step.Body, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
	if err != nil {
		return fmt.Errorf("compile step %s: %w", step.ID, err)
	}
	step.StoreKeys = storeKeys
	step.Compiled = compiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "body", storeKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "body", storeKeys, peerIDs); err != nil {
		return err
	}

	fallbackStoreKeys, fallbackCompiled, err := c.compileBody(ctx, step.FallbackBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
	if err != nil {
		return fmt.Errorf("compile fallback for step %s: %w", step.ID, err)
	}
	step.FallbackStoreKeys = fallbackStoreKeys
	step.FallbackCompiled = fallbackCompiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "fallback", fallbackStoreKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "fallback", fallbackStoreKeys, peerIDs); err != nil {
		return err
	}

	compensateStoreKeys, compensateCompiled, err := c.compileBody(ctx, step.CompensateBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
	if err != nil {
		return fmt.Errorf("compile compensation for step %s: %w", step.ID, err)
	}
	step.CompensateCompiled = compensateCompiled
	if err := validateNoForwardAsyncRefs(step.ID, index, "compensate", compensateStoreKeys, asyncStepIndexes); err != nil {
		return err
	}

	conditionStoreKeys, err := extractStoreKeys(step.Condition, knownStoreKeys, frameworkKeys)
	if err != nil {
		return fmt.Errorf("compile condition for step %s: %w", step.ID, err)
	}
	if err := validateNoForwardAsyncRefs(step.ID, index, "condition", conditionStoreKeys, asyncStepIndexes); err != nil {
		return err
	}
	if err := validateNoPeerRefs(step.ID, "condition", conditionStoreKeys, peerIDs); err != nil {
		return err
	}

	var retryStoreKeys []string
	if step.Retry != nil {
		retryStoreKeys, err = extractStoreKeys(step.Retry.When, knownStoreKeys, frameworkKeys)
		if err != nil {
			return fmt.Errorf("compile retry for step %s: %w", step.ID, err)
		}
		if err := validateNoForwardAsyncRefs(step.ID, index, "retry", retryStoreKeys, asyncStepIndexes); err != nil {
			return err
		}
		if err := validateNoPeerRefs(step.ID, "retry", retryStoreKeys, peerIDs); err != nil {
			return err
		}
	}

	step.AsyncDeps = intersectAsyncDeps(asyncStepIndexes, storeKeys, fallbackStoreKeys, compensateStoreKeys, conditionStoreKeys, retryStoreKeys)
	return nil
}

func (c *Compiler) compileBody(
	ctx context.Context,
	body string,
	knownStoreKeys []string,
	frameworkKeys map[string]struct{},
	frameworkEnv map[string]any,
	contract runtime.ResponseContract,
) ([]string, any, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil, nil
	}

	if err := ValidateResponseCalls(body, contract); err != nil {
		return nil, nil, err
	}

	storeKeys, err := ExtractStoreKeys(body, knownStoreKeys, frameworkKeys)
	if err != nil {
		return nil, nil, err
	}
	if storeKeys == nil {
		storeKeys = []string{}
	}

	templateEnv := buildStepTemplateEnv(storeKeys, frameworkEnv)
	code, err := c.interpreter.Compile(ctx, body, templateEnv)
	if err != nil {
		return nil, nil, err
	}

	return storeKeys, code, nil
}

func extractStoreKeys(source string, knownStoreKeys []string, frameworkKeys map[string]struct{}) ([]string, error) {
	storeKeys, err := ExtractStoreKeys(source, knownStoreKeys, frameworkKeys)
	if err != nil {
		return nil, err
	}
	if storeKeys == nil {
		return []string{}, nil
	}
	return storeKeys, nil
}

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

func collectKnownStoreKeys(flow *runtime.Flow) []string {
	keys := []string{"request", "input", "properties", "error", "compensation"}
	seen := map[string]struct{}{
		"request":      {},
		"input":        {},
		"properties":   {},
		"error":        {},
		"compensation": {},
	}
	if flow.Entrypoint.Type == "kafka" {
		keys = append(keys, "message")
		seen["message"] = struct{}{}
	}
	if flow.Entrypoint.Type == "cron" {
		keys = append(keys, "trigger")
		seen["trigger"] = struct{}{}
	}

	for _, node := range runtime.NormalizeFlowNodes(flow) {
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step == nil {
				continue
			}
			if _, ok := seen[node.Step.ID]; ok {
				continue
			}
			seen[node.Step.ID] = struct{}{}
			keys = append(keys, node.Step.ID)
		case runtime.FlowNodeParallel:
			if node.Parallel == nil {
				continue
			}
			for _, branch := range node.Parallel.Branches {
				if _, ok := seen[branch.ID]; ok {
					continue
				}
				seen[branch.ID] = struct{}{}
				keys = append(keys, branch.ID)
			}
		}
	}

	sort.Strings(keys[5:])
	return keys
}

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
		if node.Kind == runtime.FlowNodeStep && strings.HasPrefix(node.ID, runtime.InternalParallelNodePrefix) {
			return fmt.Errorf("step ID %q uses reserved internal prefix %q", node.ID, runtime.InternalParallelNodePrefix)
		}
	}
	return nil
}

func validateResultIDs(nodes []runtime.FlowNode) error {
	seen := map[string]struct{}{}
	for _, node := range nodes {
		switch node.Kind {
		case runtime.FlowNodeStep:
			if node.Step == nil {
				continue
			}
			if err := addResultID(seen, node.Step.ID); err != nil {
				return err
			}
		case runtime.FlowNodeParallel:
			if node.Parallel == nil {
				continue
			}
			for _, branch := range node.Parallel.Branches {
				if err := addResultID(seen, branch.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addResultID(seen map[string]struct{}, id string) error {
	if id == "" {
		return fmt.Errorf("step ID is required")
	}
	if strings.HasPrefix(id, runtime.InternalParallelNodePrefix) {
		return fmt.Errorf("step ID %q uses reserved internal prefix %q", id, runtime.InternalParallelNodePrefix)
	}
	if _, exists := seen[id]; exists {
		return fmt.Errorf("duplicate result ID %q", id)
	}
	seen[id] = struct{}{}
	return nil
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

func collectFrameworkInfo(container *runtime.Container) (map[string]struct{}, map[string]any) {
	frameworkKeys := map[string]struct{}{
		"response":      {},
		"flow":          {},
		"log":           {},
		"metric":        {},
		"sprintf":       {},
		"base64_encode": {},
		"raise":         {},
	}

	frameworkEnv := map[string]any{
		"response":      buildResponseTemplateModule(runtime.HTTPResponseContract()),
		"flow":          buildFlowTemplateModule(),
		"log":           buildLogTemplateModule(),
		"metric":        buildMetricTemplateModule(container),
		"sprintf":       fmt.Sprintf,
		"base64_encode": func(v any) string { return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", v))) },
		"raise":         func(args ...any) (any, error) { return nil, nil },
	}

	if container == nil {
		return frameworkKeys, frameworkEnv
	}

	plugins := buildPluginTemplateModules(container)
	for name, module := range plugins {
		frameworkKeys[name] = struct{}{}
		frameworkEnv[name] = module
	}

	return frameworkKeys, frameworkEnv
}

func buildFlowTemplateModule() map[string]any {
	return map[string]any{
		"call": func(name string, args map[string]any) (map[string]any, error) {
			return nil, nil
		},
	}
}

func buildStepTemplateEnv(storeKeys []string, frameworkEnv map[string]any) map[string]any {
	env := make(map[string]any, len(storeKeys)+len(frameworkEnv))
	for _, key := range storeKeys {
		env[key] = nil
	}
	for key, value := range frameworkEnv {
		env[key] = value
	}
	return env
}

func buildPluginTemplateModules(container *runtime.Container) map[string]any {
	grouped := make(map[string]map[string]any)
	container.RangeTasks(func(taskName string, _ runtime.Task) {
		parts := strings.SplitN(taskName, ".", 2)
		if len(parts) != 2 {
			return
		}
		pluginName := parts[0]
		methodName := parts[1]

		if grouped[pluginName] == nil {
			grouped[pluginName] = make(map[string]any)
		}
		grouped[pluginName][methodName] = func(args map[string]any) (map[string]any, error) {
			return nil, nil
		}
	})

	result := make(map[string]any, len(grouped))
	for name, module := range grouped {
		result[name] = module
	}
	return result
}

func buildResponseTemplateModule(contract runtime.ResponseContract) map[string]any {
	subtypes := contract.Subtypes()
	methods := make(map[string]any, len(subtypes))
	for _, subtype := range subtypes {
		subtype := subtype
		methods[subtype] = func(args ...any) error {
			_, err := contract.ValidateArgs(subtype, args)
			return err
		}
	}
	return methods
}

func responseContractForFlow(flow *runtime.Flow) runtime.ResponseContract {
	if !flow.ResponseContract.IsZero() {
		return flow.ResponseContract
	}
	if contract, ok := runtime.BuiltInResponseContract(flow.Entrypoint.Type); ok {
		return contract
	}
	return runtime.ResponseContract{}
}

func buildLogTemplateModule() map[string]any {
	return map[string]any{
		"debug": func(args ...any) error { return nil },
		"info":  func(args ...any) error { return nil },
		"warn":  func(args ...any) error { return nil },
		"error": func(args ...any) error { return nil },
	}
}

func buildMetricTemplateModule(container *runtime.Container) map[string]any {
	module := map[string]any{
		"counter":       func(args ...any) error { return nil },
		"updowncounter": func(args ...any) error { return nil },
		"histogram":     func(args ...any) error { return nil },
		"gauge":         func(args ...any) error { return nil },
	}

	if container == nil || container.Metrics() == nil {
		return module
	}

	for name, decl := range container.Metrics().UserDeclarations() {
		switch decl.Type {
		case "counter":
			module[name] = map[string]any{"inc": func(args ...any) error { return nil }}
		case "updowncounter":
			module[name] = map[string]any{"add": func(args ...any) error { return nil }}
		case "histogram":
			module[name] = map[string]any{"observe": func(args ...any) error { return nil }}
		case "gauge":
			module[name] = map[string]any{"set": func(args ...any) error { return nil }}
		}
	}

	return module
}
