package dsl

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/runtime"
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

	for taskName, task := range exec.Container.Tasks {
		parts := strings.SplitN(taskName, ".", 2)
		if len(parts) != 2 {
			continue
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
	}

	result := make(map[string]any, len(grouped))
	for k, v := range grouped {
		result[k] = v
	}
	return result
}

// BuildResponseGlobals creates the "response" global module for Risor DSL code.
// Both step bodies and return bodies use this — response.*() calls set
// execution.ResponseDescriptor, and the ReturnHandler dispatches it to gin.
func BuildResponseGlobals(exec *runtime.Execution) map[string]any {
	responseMethods := make(map[string]any)

	for handlerName := range exec.Container.ResponseHandlers.All() {
		parts := strings.SplitN(handlerName, ".", 2)
		if len(parts) != 2 {
			continue
		}
		methodName := parts[1]

		hn := handlerName
		responseMethods[methodName] = func(args ...any) error {
			argsMap, err := normalizeResponseArgs(args)
			if err != nil {
				return err
			}
			exec.ResponseDescriptor = &runtime.ResponseDescriptor{
				HandlerName: hn,
				Args:        argsMap,
			}
			return nil
		}
	}

	return map[string]any{
		"response": responseMethods,
	}
}

// normalizeResponseArgs converts the variadic args from a Risor function call
// into a map[string]any suitable for response handlers.
// Supports: response.json({status: 200, body: {...}}) → single map arg
// Also: response.json(200, {id: "..."}) → {status: 200, body: {...}}
func normalizeResponseArgs(args []any) (map[string]any, error) {
	if len(args) == 0 {
		return map[string]any{}, nil
	}

	// Single map argument
	if len(args) == 1 {
		if m, ok := args[0].(map[string]any); ok {
			return m, nil
		}
		return nil, fmt.Errorf("expected map argument, got %T", args[0])
	}

	// Two arguments: status code + body
	if len(args) == 2 {
		result := map[string]any{}
		result["status"] = args[0]
		result["body"] = args[1]
		return result, nil
	}

	return nil, fmt.Errorf("expected 1 or 2 arguments, got %d", len(args))
}
