package pluginexec

import (
	"fmt"
	"reflect"

	"github.com/BDNK1/sflowg/core/internal/configutil"
)

func extractError(v reflect.Value) (error, bool) {
	if v.IsNil() {
		return nil, true
	}
	err, ok := v.Interface().(error)
	return err, ok
}

func CallTask(binding TaskBinding, exec any, args map[string]any) (out map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("plugin %s.%s panicked: %v", binding.PluginName, binding.MethodName, r)
		}
	}()

	input, err := prepareInput(binding, args)
	if err != nil {
		return nil, err
	}

	results := binding.method.Func.Call([]reflect.Value{
		binding.plugin,
		reflect.ValueOf(exec),
		input,
	})

	if len(results) < 2 {
		return nil, fmt.Errorf("plugin %s.%s returned %d values, expected 2", binding.PluginName, binding.MethodName, len(results))
	}

	taskErr, ok := extractError(results[1])
	if !ok {
		return nil, fmt.Errorf("plugin %s.%s second return value is not an error", binding.PluginName, binding.MethodName)
	}
	if taskErr != nil {
		return nil, taskErr
	}

	output := results[0].Interface()
	return convertOutput(binding, output)
}

func prepareInput(binding TaskBinding, args map[string]any) (reflect.Value, error) {
	if binding.InputType.Kind() != reflect.Struct {
		return reflect.ValueOf(args), nil
	}

	inputPtr := reflect.New(binding.InputType)
	if err := configutil.MapToStruct(args, inputPtr.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("invalid input for task %s: %w", binding.MethodName, err)
	}
	if err := configutil.ValidateStruct(inputPtr.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("validation failed for task %s: %w", binding.MethodName, err)
	}
	return inputPtr.Elem(), nil
}

func convertOutput(binding TaskBinding, output any) (map[string]any, error) {
	if binding.OutputType.Kind() != reflect.Struct {
		m, ok := output.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("task %s output is %T, expected map[string]any", binding.MethodName, output)
		}
		return m, nil
	}

	resultMap, err := configutil.StructToMap(output)
	if err != nil {
		return nil, fmt.Errorf("failed to convert output for task %s: %w", binding.MethodName, err)
	}
	return resultMap, nil
}
