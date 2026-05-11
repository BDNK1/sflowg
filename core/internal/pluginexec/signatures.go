package pluginexec

import (
	"fmt"
	"reflect"
	"strings"
)

var (
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
	mapStringAny = reflect.TypeOf(map[string]any(nil))
)

var reservedLifecycleMethods = map[string]string{
	"Initialize": "func(plugin.Logger) error",
	"Shutdown":   "func(plugin.Logger) error",
}

type TaskBinding struct {
	TaskName   string
	PluginName string
	MethodName string
	InputType  reflect.Type
	OutputType reflect.Type
	plugin     reflect.Value
	method     reflect.Method
}

type SignatureIssue struct {
	MethodName string
	Reason     string
}

func Discover(pluginName string, plugin any) ([]TaskBinding, []SignatureIssue) {
	pluginType := reflect.TypeOf(plugin)
	pluginValue := reflect.ValueOf(plugin)

	var tasks []TaskBinding
	var issues []SignatureIssue

	for i := 0; i < pluginType.NumMethod(); i++ {
		method := pluginType.Method(i)
		if !method.IsExported() {
			continue
		}

		if expected, reserved := reservedLifecycleMethods[method.Name]; reserved {
			if reason := lifecycleSignatureReason(method); reason != "" {
				issues = append(issues, SignatureIssue{
					MethodName: method.Name,
					Reason:     fmt.Sprintf("expected %s, got %s (%s)", expected, method.Type, reason),
				})
			}
			continue
		}

		if isValidTaskSignature(method.Type) {
			tasks = append(tasks, TaskBinding{
				TaskName:   fmt.Sprintf("%s.%s", pluginName, lowerFirst(method.Name)),
				PluginName: pluginName,
				MethodName: lowerFirst(method.Name),
				plugin:     pluginValue,
				method:     method,
				InputType:  method.Type.In(2),
				OutputType: method.Type.Out(0),
			})
			continue
		}

		if reason := taskSignatureReason(method.Type); reason != "" {
			issues = append(issues, SignatureIssue{
				MethodName: method.Name,
				Reason:     reason,
			})
		}
	}

	return tasks, issues
}

func lifecycleSignatureReason(method reflect.Method) string {
	mt := method.Type
	if mt.NumIn() != 2 || mt.NumOut() != 1 {
		return fmt.Sprintf("wrong arity NumIn=%d NumOut=%d", mt.NumIn(), mt.NumOut())
	}
	if mt.Out(0) != errorType {
		return "second return must be error"
	}
	arg := mt.In(1)
	if arg.Kind() != reflect.Interface || arg.Name() != "Logger" {
		return "first parameter must be plugin.Logger"
	}
	return ""
}

func taskSignatureReason(methodType reflect.Type) string {
	if methodType.NumIn() != 3 || methodType.NumOut() != 2 {
		return ""
	}
	if !isRuntimeExecutionPointer(methodType.In(1)) {
		return "second parameter must be *plugin.Execution"
	}
	if !isMapOrStruct(methodType.In(2)) {
		return "third parameter must be a struct or map[string]any"
	}
	if !isMapOrStruct(methodType.Out(0)) {
		return "first return must be a struct or map[string]any"
	}
	if methodType.Out(1) != errorType {
		return "second return must be error"
	}
	return ""
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

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func isRuntimeExecutionPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr &&
		t.Elem().Name() == "Execution" &&
		t.Elem().PkgPath() == "github.com/BDNK1/sflowg/core"
}
