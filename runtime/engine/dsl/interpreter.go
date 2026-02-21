package dsl

import (
	"context"
	"fmt"
	"reflect"

	"github.com/risor-io/risor"
	"github.com/risor-io/risor/object"
)

// Interpreter wraps Risor's Eval with sandboxing.
// WithoutDefaultGlobals removes os/exec/file builtins — only explicitly
// injected globals are available to flow code.
type Interpreter struct{}

func (i *Interpreter) Eval(ctx context.Context, code string, globals map[string]any) (any, error) {
	// Pre-convert globals to Risor object types.
	// Raw Go funcs and nested maps containing funcs would panic in the VM
	// because object.AsObjects doesn't handle reflect.Func.
	converted := convertGlobals(globals)

	result, err := risor.Eval(ctx, code,
		risor.WithoutDefaultGlobals(),
		risor.WithGlobals(converted),
	)
	if err != nil {
		return nil, err
	}
	return objectToGo(result), nil
}

// convertGlobals converts a Go map into a Risor-safe globals map.
// Functions become *object.Builtin, nested maps become *object.Module,
// and primitive types are left as-is (Risor's VM handles them via FromGoType).
func convertGlobals(globals map[string]any) map[string]any {
	result := make(map[string]any, len(globals))
	for k, v := range globals {
		result[k] = goToRisor(k, v)
	}
	return result
}

// goToRisor converts a single Go value to a Risor-compatible type.
func goToRisor(name string, v any) any {
	if v == nil {
		return nil
	}

	// Already a Risor object? Pass through.
	if _, ok := v.(object.Object); ok {
		return v
	}

	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Func:
		return wrapGoFunc(name, v)

	case reflect.Map:
		// Check if any values are functions — if so, create a Module
		if m, ok := v.(map[string]any); ok {
			hasFuncs := false
			for _, val := range m {
				if val != nil && reflect.TypeOf(val).Kind() == reflect.Func {
					hasFuncs = true
					break
				}
			}
			if hasFuncs {
				return mapToModule(name, m)
			}
			// Pure data map — convert recursively so nested maps with funcs are handled
			converted := make(map[string]any, len(m))
			for k, val := range m {
				converted[k] = goToRisor(k, val)
			}
			return converted
		}
		return v

	default:
		// Primitive types (string, int, float, bool, etc.) — Risor handles these
		return v
	}
}

// wrapGoFunc wraps an arbitrary Go function as a Risor *object.Builtin.
// The wrapper converts Risor Object args to Go values, calls the function
// via reflection, and converts the return value back.
func wrapGoFunc(name string, fn any) *object.Builtin {
	fnValue := reflect.ValueOf(fn)
	fnType := fnValue.Type()

	return object.NewBuiltin(name, func(ctx context.Context, args ...object.Object) object.Object {
		// Convert Risor args to Go values
		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			goVal := objectToGo(arg)
			if i < fnType.NumIn() {
				expectedType := fnType.In(i)
				goArgs[i] = convertToExpectedType(goVal, expectedType)
			} else if fnType.IsVariadic() && i >= fnType.NumIn()-1 {
				elemType := fnType.In(fnType.NumIn() - 1).Elem()
				goArgs[i] = convertToExpectedType(goVal, elemType)
			} else {
				goArgs[i] = reflect.ValueOf(goVal)
			}
		}

		// Call the Go function
		var results []reflect.Value
		if fnType.IsVariadic() {
			results = fnValue.Call(goArgs)
		} else {
			results = fnValue.Call(goArgs)
		}

		// Handle return values
		if len(results) == 0 {
			return object.Nil
		}

		// Check for error in last return value
		lastIdx := len(results) - 1
		if fnType.NumOut() > 0 && fnType.Out(lastIdx).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if !results[lastIdx].IsNil() {
				errVal := results[lastIdx].Interface().(error)
				return object.NewError(errVal)
			}
			// If there's a non-error return value, use it
			if len(results) > 1 {
				return goValueToObject(results[0].Interface())
			}
			return object.Nil
		}

		// Single return value
		return goValueToObject(results[0].Interface())
	})
}

// convertToExpectedType converts a Go value to the expected reflect.Type.
func convertToExpectedType(val any, expected reflect.Type) reflect.Value {
	if val == nil {
		return reflect.Zero(expected)
	}
	actual := reflect.ValueOf(val)
	if actual.Type().AssignableTo(expected) {
		return actual
	}
	if actual.Type().ConvertibleTo(expected) {
		return actual.Convert(expected)
	}
	// Best effort — pass as-is
	return actual
}

// goValueToObject converts a Go value to a Risor object.Object.
func goValueToObject(v any) object.Object {
	if v == nil {
		return object.Nil
	}
	obj := object.FromGoType(v)
	if obj == nil {
		return object.Nil
	}
	return obj
}

// mapToModule converts a map[string]any (with function values) to a Risor Module.
// This enables `http.request(...)` syntax — `http` is the module, `request` is a builtin.
func mapToModule(name string, m map[string]any) *object.Module {
	contents := make(map[string]object.Object, len(m))
	for k, v := range m {
		if v == nil {
			contents[k] = object.Nil
			continue
		}
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Func {
			contents[k] = wrapGoFunc(fmt.Sprintf("%s.%s", name, k), v)
		} else {
			contents[k] = goValueToObject(v)
		}
	}
	return object.NewBuiltinsModule(name, contents)
}

// objectToGo recursively converts a Risor object.Object to a native Go value.
func objectToGo(obj object.Object) any {
	if obj == nil {
		return nil
	}

	switch o := obj.(type) {
	case *object.Map:
		goMap := make(map[string]any)
		for k, v := range o.Value() {
			goMap[k] = objectToGo(v)
		}
		return goMap
	case *object.List:
		items := o.Value()
		goSlice := make([]any, len(items))
		for i, v := range items {
			goSlice[i] = objectToGo(v)
		}
		return goSlice
	case *object.NilType:
		return nil
	default:
		// For String, Int, Float, Bool, etc. — Interface() returns the native Go value
		return obj.Interface()
	}
}
