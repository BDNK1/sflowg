package dsl

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/BDNK1/sflowg/core"
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

	for i := range flow.Steps {
		step := &flow.Steps[i]

		storeKeys, compiled, err := c.compileBody(ctx, step.Body, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
		if err != nil {
			return fmt.Errorf("compile step %s: %w", step.ID, err)
		}
		step.StoreKeys = storeKeys
		step.Compiled = compiled
		if err := validateNoForwardAsyncRefs(step.ID, i, "body", storeKeys, asyncStepIndexes); err != nil {
			return err
		}

		fallbackStoreKeys, fallbackCompiled, err := c.compileBody(ctx, step.FallbackBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
		if err != nil {
			return fmt.Errorf("compile fallback for step %s: %w", step.ID, err)
		}
		step.FallbackStoreKeys = fallbackStoreKeys
		step.FallbackCompiled = fallbackCompiled
		if err := validateNoForwardAsyncRefs(step.ID, i, "fallback", fallbackStoreKeys, asyncStepIndexes); err != nil {
			return err
		}

		compensateStoreKeys, compensateCompiled, err := c.compileBody(ctx, step.CompensateBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
		if err != nil {
			return fmt.Errorf("compile compensation for step %s: %w", step.ID, err)
		}
		step.CompensateCompiled = compensateCompiled
		if err := validateNoForwardAsyncRefs(step.ID, i, "compensate", compensateStoreKeys, asyncStepIndexes); err != nil {
			return err
		}

		conditionStoreKeys, err := extractStoreKeys(step.Condition, knownStoreKeys, frameworkKeys)
		if err != nil {
			return fmt.Errorf("compile condition for step %s: %w", step.ID, err)
		}
		if err := validateNoForwardAsyncRefs(step.ID, i, "condition", conditionStoreKeys, asyncStepIndexes); err != nil {
			return err
		}

		var retryStoreKeys []string
		if step.Retry != nil {
			retryStoreKeys, err = extractStoreKeys(step.Retry.When, knownStoreKeys, frameworkKeys)
			if err != nil {
				return fmt.Errorf("compile retry for step %s: %w", step.ID, err)
			}
			if err := validateNoForwardAsyncRefs(step.ID, i, "retry", retryStoreKeys, asyncStepIndexes); err != nil {
				return err
			}
		}

		step.AsyncDeps = intersectAsyncDeps(asyncStepIndexes, storeKeys, fallbackStoreKeys, compensateStoreKeys, conditionStoreKeys, retryStoreKeys)
	}

	_, onErrorCompiled, err := c.compileBody(ctx, flow.OnErrorBody, knownStoreKeys, frameworkKeys, frameworkEnv, contract)
	if err != nil {
		return fmt.Errorf("compile on_error: %w", err)
	}
	flow.OnErrorCompiled = onErrorCompiled

	returnStoreKeys, err := extractStoreKeys(flow.Return.Body, knownStoreKeys, frameworkKeys)
	if err != nil {
		return fmt.Errorf("compile return: %w", err)
	}
	if err := validateNoForwardAsyncRefs("__return", len(flow.Steps), "return", returnStoreKeys, asyncStepIndexes); err != nil {
		return err
	}

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
	for i, step := range flow.Steps {
		if step.Async {
			indexes[step.ID] = i
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

	for _, step := range flow.Steps {
		if _, ok := seen[step.ID]; ok {
			continue
		}
		seen[step.ID] = struct{}{}
		keys = append(keys, step.ID)
	}

	sort.Strings(keys[5:])
	return keys
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
