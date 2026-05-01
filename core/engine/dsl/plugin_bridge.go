package dsl

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
)

// BuildPluginGlobals converts container tasks (e.g., "http.request", "postgres.get")
// into a nested map of Risor-callable Go functions, grouped by plugin prefix.
//
// Result structure:
//
//	{
//	  "http":     { "request": func(args map[string]any) (map[string]any, error) },
//	  "postgres": { "get": func(...), "exec": func(...) },
//	}
//
// Risor auto-wraps Go functions, so `http.request({url: "..."})` in DSL code
// performs map attribute access on "http" then calls the "request" function.
func BuildPluginGlobals(exec *runtime.Execution) map[string]any {
	grouped := make(map[string]map[string]any)

	exec.Container.RangeTasks(func(taskName string, task runtime.Task) {
		parts := strings.SplitN(taskName, ".", 2)
		if len(parts) != 2 {
			return
		}
		pluginName := parts[0]
		methodName := parts[1]

		if grouped[pluginName] == nil {
			grouped[pluginName] = make(map[string]any)
		}

		// Capture task in closure for Risor to call
		t := task
		e := exec
		grouped[pluginName][methodName] = func(args map[string]any) (map[string]any, error) {
			return t.Execute(e, args)
		}
	})

	result := make(map[string]any, len(grouped))
	for k, v := range grouped {
		result[k] = v
	}
	return result
}

// BuildResponseGlobals creates the "response" global module for Risor DSL code.
// Both step bodies and return bodies use this; response.*() calls set a
// descriptor for the active transport to dispatch.
func BuildResponseGlobals(exec *runtime.Execution) map[string]any {
	responseMethods := make(map[string]any)

	contract := responseContractForFlow(exec.Flow)
	subtypes := contract.Subtypes()

	for _, subtype := range subtypes {
		subtype := subtype
		entrypointType := contract.EntrypointType
		handlerName := entrypointType + "." + subtype
		responseMethods[subtype] = func(args ...any) error {
			argsMap, err := contract.ValidateArgs(subtype, args)
			if err != nil {
				return err
			}
			exec.State().SetResponse(&runtime.ResponseDescriptor{
				Subtype:     subtype,
				HandlerName: handlerName,
				Args:        argsMap,
			})
			return nil
		}
	}

	return map[string]any{
		"response": responseMethods,
	}
}

func BuildFlowCallGlobals(invoker runtime.SubflowInvoker, exec *runtime.Execution) map[string]any {
	return map[string]any{
		"flow": map[string]any{
			"call": func(name string, args map[string]any) (map[string]any, error) {
				if invoker == nil {
					return nil, fmt.Errorf("flow.call is not available")
				}
				target, ok := invoker.LookupFlow(name)
				if !ok {
					return nil, &runtime.FlowError{
						Type:    runtime.ErrorTypePermanent,
						Code:    string(runtime.ErrorCodeRuntimeError),
						Message: fmt.Sprintf("undefined subflow %q", name),
					}
				}
				if target.Entrypoint.Type != "flow" {
					return nil, &runtime.FlowError{
						Type:    runtime.ErrorTypePermanent,
						Code:    string(runtime.ErrorCodeRuntimeError),
						Message: fmt.Sprintf("flow.call target %q must be entrypoint.flow, got %q", name, target.Entrypoint.Type),
					}
				}
				return invoker.InvokeSubflow(exec, target, args)
			},
		},
	}
}
