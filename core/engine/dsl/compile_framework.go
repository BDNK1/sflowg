package dsl

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
)

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
