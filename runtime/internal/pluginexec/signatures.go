package pluginexec

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	errorType     = reflect.TypeOf((*error)(nil)).Elem()
	mapStringAny  = reflect.TypeOf(map[string]any(nil))
	ginContextPtr = reflect.TypeOf((*gin.Context)(nil))
)

type TaskBinding struct {
	TaskName   string
	PluginName string
	MethodName string
	InputType  reflect.Type
	OutputType reflect.Type
	plugin     reflect.Value
	method     reflect.Method
}

type ResponseBinding struct {
	HandlerName string
	plugin      reflect.Value
	method      reflect.Method
}

func Discover(pluginName string, plugin any) ([]TaskBinding, []ResponseBinding) {
	pluginType := reflect.TypeOf(plugin)
	pluginValue := reflect.ValueOf(plugin)

	var tasks []TaskBinding
	var responses []ResponseBinding

	for i := 0; i < pluginType.NumMethod(); i++ {
		method := pluginType.Method(i)
		if !method.IsExported() {
			continue
		}

		lowerMethodName := lowerFirst(method.Name)

		if isValidTaskSignature(method.Type) {
			tasks = append(tasks, TaskBinding{
				TaskName:   fmt.Sprintf("%s.%s", pluginName, lowerMethodName),
				PluginName: pluginName,
				MethodName: lowerMethodName,
				plugin:     pluginValue,
				method:     method,
				InputType:  method.Type.In(2),
				OutputType: method.Type.Out(0),
			})
			continue
		}

		if isValidResponseHandlerSignature(method.Type) {
			responses = append(responses, ResponseBinding{
				HandlerName: fmt.Sprintf("%s.%s", pluginName, lowerMethodName),
				plugin:      pluginValue,
				method:      method,
			})
		}
	}

	return tasks, responses
}

func isValidTaskSignature(methodType reflect.Type) bool {
	if methodType.NumIn() != 3 || methodType.NumOut() != 2 {
		return false
	}

	if !isRuntimeExecutionPointer(methodType.In(1)) {
		return false
	}

	if !isMapOrStruct(methodType.In(2)) || !isMapOrStruct(methodType.Out(0)) {
		return false
	}

	return methodType.Out(1) == errorType
}

func isMapOrStruct(t reflect.Type) bool {
	if t.Kind() == reflect.Map {
		return t == mapStringAny
	}
	return t.Kind() == reflect.Struct
}

func isValidResponseHandlerSignature(methodType reflect.Type) bool {
	if methodType.NumIn() != 4 || methodType.NumOut() != 1 {
		return false
	}

	if methodType.In(1) != ginContextPtr {
		return false
	}

	if !isRuntimeExecutionPointer(methodType.In(2)) {
		return false
	}

	if methodType.In(3) != mapStringAny {
		return false
	}

	return methodType.Out(0) == errorType
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func isRuntimeExecutionPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr &&
		t.Elem().Name() == "Execution" &&
		t.Elem().PkgPath() == "github.com/BDNK1/sflowg/runtime"
}
