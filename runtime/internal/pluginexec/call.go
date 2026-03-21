package pluginexec

import (
	"fmt"
	"reflect"

	"github.com/BDNK1/sflowg/runtime/internal/configutil"
	"github.com/gin-gonic/gin"
)

func extractError(v reflect.Value) error {
	if !v.IsNil() {
		return v.Interface().(error)
	}
	return nil
}

func CallTask(binding TaskBinding, exec any, args map[string]any) (map[string]any, error) {
	input, err := prepareInput(binding, args)
	if err != nil {
		return nil, err
	}

	results := binding.method.Func.Call([]reflect.Value{
		binding.plugin,
		reflect.ValueOf(exec),
		input,
	})

	output := results[0].Interface()
	if err := extractError(results[1]); err != nil {
		return nil, err
	}

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
		return output.(map[string]any), nil
	}

	resultMap, err := configutil.StructToMap(output)
	if err != nil {
		return nil, fmt.Errorf("failed to convert output for task %s: %w", binding.MethodName, err)
	}
	return resultMap, nil
}

func CallResponseHandler(binding ResponseBinding, c *gin.Context, exec any, args map[string]any) error {
	results := binding.method.Func.Call([]reflect.Value{
		binding.plugin,
		reflect.ValueOf(c),
		reflect.ValueOf(exec),
		reflect.ValueOf(args),
	})

	return extractError(results[0])
}
