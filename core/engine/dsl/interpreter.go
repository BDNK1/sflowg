package dsl

import (
	"context"
	"fmt"
	"reflect"

	"github.com/BDNK1/sflowg/core"
	risor "github.com/deepnoodle-ai/risor/v2"
	"github.com/deepnoodle-ai/risor/v2/pkg/bytecode"
	"github.com/deepnoodle-ai/risor/v2/pkg/object"
	"github.com/deepnoodle-ai/risor/v2/pkg/op"
)

// Interpreter wraps Risor's Eval.
// v2 is sandboxed by default. Two conversion passes are applied to globals
// before evaluation:
//   - maps with Go function values → *object.Module (avoids built-in method shadowing)
//   - maps with plain values → *lenientMap (returns nil for missing attribute access)
type Interpreter struct{}

func (i *Interpreter) Eval(ctx context.Context, code string, globals map[string]any) (result any, err error) {
	defer recoverFlowError(&err)
	result, err = risor.Eval(ctx, code, risor.WithEnv(convertGlobals(globals)))
	if err != nil {
		return nil, err
	}
	if fe := asyncFutureError(globals); fe != nil {
		return nil, fe
	}
	return result, nil
}

func (i *Interpreter) Compile(ctx context.Context, source string, templateEnv map[string]any) (*bytecode.Code, error) {
	return risor.Compile(ctx, source, risor.WithEnv(convertGlobals(templateEnv)))
}

func (i *Interpreter) Run(ctx context.Context, code *bytecode.Code, globals map[string]any) (result any, err error) {
	defer recoverFlowError(&err)
	result, err = risor.Run(ctx, code, risor.WithEnv(convertGlobals(globals)))
	if err != nil {
		return nil, err
	}
	if fe := asyncFutureError(globals); fe != nil {
		return nil, fe
	}
	return result, nil
}

func recoverFlowError(err *error) {
	if r := recover(); r != nil {
		if fe, ok := r.(*runtime.FlowError); ok {
			*err = fe
			return
		}
		panic(r)
	}
}

// convertGlobals prepares the globals map for Risor evaluation.
func convertGlobals(globals map[string]any) map[string]any {
	result := make(map[string]any, len(globals))
	for k, v := range globals {
		if m, ok := v.(map[string]any); ok {
			if containsFunc(m) {
				result[k] = mapToModule(k, m)
			} else {
				result[k] = newLenientMap(m)
			}
		} else {
			result[k] = v
		}
	}
	return result
}

func containsFunc(m map[string]any) bool {
	for _, v := range m {
		if v != nil && reflect.TypeOf(v).Kind() == reflect.Func {
			return true
		}
	}
	return false
}

// mapToModule converts a map whose values include Go functions into a Risor
// *object.Module. This prevents name collisions with built-in map methods
// (e.g. .get, .keys) that would shadow function keys of the same name.
func mapToModule(name string, m map[string]any) *object.Module {
	contents := make(map[string]object.Object, len(m))
	for k, v := range m {
		qualifiedName := fmt.Sprintf("%s.%s", name, k)
		if v == nil {
			contents[k] = object.Nil
		} else if reflect.TypeOf(v).Kind() == reflect.Func {
			contents[k] = wrapGoFunc(qualifiedName, v)
		} else if nm, ok := v.(map[string]any); ok && containsFunc(nm) {
			contents[k] = mapToModule(qualifiedName, nm)
		} else {
			contents[k] = anyToObject(v)
		}
	}
	return object.NewBuiltinsModule(name, contents)
}

func wrapGoFunc(name string, fn any) *object.Builtin {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()
	errType := reflect.TypeOf((*error)(nil)).Elem()
	isVariadic := fnType.IsVariadic()
	numIn := fnType.NumIn()

	return object.NewBuiltin(name, func(ctx context.Context, args ...object.Object) (result object.Object, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = nil
				err = fmt.Errorf("dsl builtin %q panicked: %v", name, r)
			}
		}()

		if isVariadic {
			if len(args) < numIn-1 {
				return nil, fmt.Errorf("dsl builtin %q: expected at least %d arguments, got %d", name, numIn-1, len(args))
			}
		} else if len(args) != numIn {
			return nil, fmt.Errorf("dsl builtin %q: expected %d arguments, got %d", name, numIn, len(args))
		}

		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			goVal := arg.Interface()
			expected := fnType.In(min(i, numIn-1))
			if isVariadic && i >= numIn-1 {
				expected = fnType.In(numIn - 1).Elem()
			}
			converted, convErr := convertArg(goVal, expected)
			if convErr != nil {
				return nil, fmt.Errorf("dsl builtin %q: arg %d: %w", name, i, convErr)
			}
			goArgs[i] = converted
		}

		var results []reflect.Value
		if isVariadic {
			results = fnVal.CallSlice(buildVariadicArgs(goArgs, numIn, fnType.In(numIn-1)))
		} else {
			results = fnVal.Call(goArgs)
		}
		if len(results) == 0 {
			return object.Nil, nil
		}

		lastIdx := len(results) - 1
		if fnType.NumOut() > 0 && fnType.Out(lastIdx).Implements(errType) {
			if !results[lastIdx].IsNil() {
				if taskErr, ok := results[lastIdx].Interface().(error); ok {
					return nil, taskErr
				}
				return nil, fmt.Errorf("dsl builtin %q: last return is not an error", name)
			}
			if len(results) > 1 {
				return anyToObject(results[0].Interface()), nil
			}
			return object.Nil, nil
		}
		return anyToObject(results[0].Interface()), nil
	})
}

func buildVariadicArgs(goArgs []reflect.Value, numIn int, variadicType reflect.Type) []reflect.Value {
	fixed := numIn - 1
	out := make([]reflect.Value, fixed+1)
	for i := 0; i < fixed; i++ {
		out[i] = goArgs[i]
	}
	slice := reflect.MakeSlice(variadicType, len(goArgs)-fixed, len(goArgs)-fixed)
	for i := fixed; i < len(goArgs); i++ {
		slice.Index(i - fixed).Set(goArgs[i])
	}
	out[fixed] = slice
	return out
}

func convertArg(val any, expected reflect.Type) (reflect.Value, error) {
	if val == nil {
		return reflect.Zero(expected), nil
	}
	actual := reflect.ValueOf(val)
	if actual.Type().AssignableTo(expected) {
		return actual, nil
	}
	if actual.Type().ConvertibleTo(expected) {
		return actual.Convert(expected), nil
	}
	return reflect.Value{}, fmt.Errorf("cannot convert %s to %s", actual.Type(), expected)
}

// anyToObject converts a Go value to a Risor Object.
// Maps become *lenientMap (nil for missing keys). Primitives use FromGoType.
func anyToObject(v any) object.Object {
	if v == nil {
		return object.Nil
	}
	if m, ok := v.(map[string]any); ok {
		return newLenientMap(m)
	}
	obj := object.FromGoType(v)
	if obj == nil {
		return object.Nil
	}
	return obj
}

// lenientMap embeds *object.Map to inherit all map methods and the full
// Object interface. Only GetAttr is overridden: missing keys return nil
// instead of (nil, false) which would cause Risor to throw "attribute not found".
//
// This allows conditions like `request.body.customer_email == nil` to work
// safely when the field is absent from the request. Use defined() to
// distinguish between a field sent as nil vs a field not sent at all.
type lenientMap struct {
	*object.Map
}

func newLenientMap(data map[string]any) *lenientMap {
	items := make(map[string]object.Object, len(data))
	for k, v := range data {
		items[k] = anyToObject(v)
	}
	return &lenientMap{Map: object.NewMap(items)}
}

// GetAttr overrides *object.Map's GetAttr to return nil for missing keys
// instead of (nil, false). Built-in map methods (.get, .keys, etc.) are
// still resolved first via the embedded Map.
func (m *lenientMap) GetAttr(name string) (object.Object, bool) {
	obj, ok := m.Map.GetAttr(name)
	if ok {
		return obj, true
	}
	return object.Nil, true
}

// RunOperation overrides the embedded map's operation to surface errors clearly.
func (m *lenientMap) RunOperation(opType op.BinaryOpType, right object.Object) (object.Object, error) {
	return nil, fmt.Errorf("map does not support operation %v", opType)
}
