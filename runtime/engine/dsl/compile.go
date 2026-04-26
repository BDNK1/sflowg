package dsl

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/BDNK1/sflowg/runtime"
)

type Compiler struct {
	interpreter *Interpreter
}

func NewCompiler() *Compiler {
	return &Compiler{interpreter: &Interpreter{}}
}

func (c *Compiler) CompileFlow(ctx context.Context, flow *runtime.Flow, container *runtime.Container) error {
	frameworkKeys, frameworkEnv := collectFrameworkInfo(container)
	frameworkEnv["response"] = buildResponseTemplateModule(flow.ResponseSubtypes)
	knownStoreKeys := collectKnownStoreKeys(flow)

	for i := range flow.Steps {
		step := &flow.Steps[i]

		storeKeys, compiled, err := c.compileBody(ctx, step.Body, knownStoreKeys, frameworkKeys, frameworkEnv, flow.ResponseSubtypes)
		if err != nil {
			return fmt.Errorf("compile step %s: %w", step.ID, err)
		}
		step.StoreKeys = storeKeys
		step.Compiled = compiled

		fallbackStoreKeys, fallbackCompiled, err := c.compileBody(ctx, step.FallbackBody, knownStoreKeys, frameworkKeys, frameworkEnv, flow.ResponseSubtypes)
		if err != nil {
			return fmt.Errorf("compile fallback for step %s: %w", step.ID, err)
		}
		step.FallbackStoreKeys = fallbackStoreKeys
		step.FallbackCompiled = fallbackCompiled

		_, compensateCompiled, err := c.compileBody(ctx, step.CompensateBody, knownStoreKeys, frameworkKeys, frameworkEnv, flow.ResponseSubtypes)
		if err != nil {
			return fmt.Errorf("compile compensation for step %s: %w", step.ID, err)
		}
		step.CompensateCompiled = compensateCompiled
	}

	_, onErrorCompiled, err := c.compileBody(ctx, flow.OnErrorBody, knownStoreKeys, frameworkKeys, frameworkEnv, flow.ResponseSubtypes)
	if err != nil {
		return fmt.Errorf("compile on_error: %w", err)
	}
	flow.OnErrorCompiled = onErrorCompiled

	return nil
}

func (c *Compiler) compileBody(
	ctx context.Context,
	body string,
	knownStoreKeys []string,
	frameworkKeys map[string]struct{},
	frameworkEnv map[string]any,
	responseSubtypes []string,
) ([]string, any, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil, nil
	}

	if err := ValidateResponseCalls(body, responseSubtypes); err != nil {
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

func collectKnownStoreKeys(flow *runtime.Flow) []string {
	keys := []string{"request", "properties", "error", "compensation"}
	seen := map[string]struct{}{
		"request":      {},
		"properties":   {},
		"error":        {},
		"compensation": {},
	}

	for _, step := range flow.Steps {
		if _, ok := seen[step.ID]; ok {
			continue
		}
		seen[step.ID] = struct{}{}
		keys = append(keys, step.ID)
	}

	sort.Strings(keys[4:])
	return keys
}

func collectFrameworkInfo(container *runtime.Container) (map[string]struct{}, map[string]any) {
	frameworkKeys := map[string]struct{}{
		"response":      {},
		"log":           {},
		"metric":        {},
		"sprintf":       {},
		"base64_encode": {},
		"raise":         {},
	}

	frameworkEnv := map[string]any{
		"response":      buildResponseTemplateModule(nil),
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

func buildResponseTemplateModule(subtypes []string) map[string]any {
	if len(subtypes) == 0 {
		subtypes = []string{"json"}
	}
	methods := make(map[string]any, len(subtypes))
	for _, subtype := range subtypes {
		methods[subtype] = func(args ...any) error { return nil }
	}
	return methods
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
