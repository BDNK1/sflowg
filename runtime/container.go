package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

// Interface type constants for plugin capabilities
const (
	InterfaceInitializer = "Initializer"
	InterfaceShutdowner  = "Shutdowner"
)

type Container struct {
	Tasks              map[string]Task
	ResponseHandlers   *ResponseHandlerRegistry
	plugins            map[string]any   // Plugin instances (name -> plugin)
	pluginsByInterface map[string][]any // Interface name -> plugins implementing that interface
}

func NewContainer() *Container {
	return &Container{
		Tasks:              make(map[string]Task),
		ResponseHandlers:   NewResponseHandlerRegistry(),
		plugins:            make(map[string]any),
		pluginsByInterface: make(map[string][]any),
	}
}

func (c *Container) GetTask(name string) Task {
	task, ok := c.Tasks[name]
	if !ok {
		return nil
	}
	return task
}

func (c *Container) SetTask(name string, task Task) {
	c.Tasks[name] = task
}

// RegisterPlugin registers a plugin instance and auto-discovers its tasks and interfaces
func (c *Container) RegisterPlugin(pluginName string, plugin any) error {
	if plugin == nil {
		return fmt.Errorf("plugin cannot be nil")
	}

	// Store plugin instance
	c.plugins[pluginName] = plugin

	// Detect and register plugin interfaces (do this once during registration)
	c.detectPluginInterfaces(plugin)

	// Discover and register all tasks and response handlers from plugin methods
	pluginType := reflect.TypeOf(plugin)
	pluginValue := reflect.ValueOf(plugin)

	for i := 0; i < pluginType.NumMethod(); i++ {
		method := pluginType.Method(i)

		// Skip unexported methods
		if !method.IsExported() {
			continue
		}

		// Check if method has valid task signature:
		// func (p *Plugin) TaskName(exec *Execution, args map[string]any) (map[string]any, error)
		if isValidTaskSignature(method.Type) {
			// Create task name: plugin_name.method_name (lowercase)
			taskName := fmt.Sprintf("%s.%s", pluginName, toLowerFirst(method.Name))

			// Create task executor wrapper
			taskExecutor := createTaskExecutor(pluginValue, method)

			// Register task
			c.Tasks[taskName] = taskExecutor
			continue
		}

		// Check if method has valid response handler signature:
		// func (p *Plugin) HandlerName(c *gin.Context, exec *Execution, args map[string]any) error
		if isValidResponseHandlerSignature(method.Type) {
			// Create handler name: plugin_name.method_name (lowercase)
			handlerName := fmt.Sprintf("%s.%s", pluginName, toLowerFirst(method.Name))

			// Create response handler wrapper
			handler := createResponseHandlerWrapper(pluginValue, method)

			// Register response handler
			c.ResponseHandlers.Register(handlerName, handler)
		}
	}

	return nil
}

// detectPluginInterfaces detects which interfaces a plugin implements and registers them
func (c *Container) detectPluginInterfaces(plugin any) {
	// Check Initializer interface (plugin.Initializer is a type alias to runtime.Initializer)
	if _, ok := plugin.(Initializer); ok {
		c.pluginsByInterface[InterfaceInitializer] = append(
			c.pluginsByInterface[InterfaceInitializer],
			plugin,
		)
	}

	// Check Shutdowner interface (plugin.Shutdowner is a type alias to runtime.Shutdowner)
	if _, ok := plugin.(Shutdowner); ok {
		c.pluginsByInterface[InterfaceShutdowner] = append(
			c.pluginsByInterface[InterfaceShutdowner],
			plugin,
		)
	}
}

// GetPlugin returns a plugin instance by name (for Phase 1 manual lookup)
func (c *Container) GetPlugin(name string) any {
	return c.plugins[name]
}

// Initialize calls Initialize on all plugins implementing Initializer interface.
// Uses fail-fast approach (panics on any error).
func (c *Container) Initialize(ctx context.Context) error {
	initializerPlugins := c.pluginsByInterface[InterfaceInitializer]

	for i, p := range initializerPlugins {
		initializer := p.(Initializer)
		if err := initializer.Initialize(); err != nil {
			panic(fmt.Sprintf("plugin #%d initialization failed: %v", i, err))
		}
	}
	return nil
}

// Shutdown calls Shutdown on all plugins implementing Shutdowner interface.
// Plugins are shut down in reverse order of registration.
func (c *Container) Shutdown(ctx context.Context) error {
	shutdownerPlugins := c.pluginsByInterface[InterfaceShutdowner]

	var errors []error
	for i := len(shutdownerPlugins) - 1; i >= 0; i-- {
		shutdowner := shutdownerPlugins[i].(Shutdowner)
		if err := shutdowner.Shutdown(); err != nil {
			errors = append(errors, fmt.Errorf("plugin #%d shutdown failed: %w", i, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// isValidTaskSignature checks if method has valid task signature
// Valid signatures:
//   - Map-based: func(exec *Execution, args map[string]any) (map[string]any, error)
//   - Typed: func(exec *Execution, input TInput) (TOutput, error) where T are structs
func isValidTaskSignature(methodType reflect.Type) bool {
	// Must have 3 inputs: receiver, *Execution, input
	if methodType.NumIn() != 3 {
		return false
	}

	// Must have 2 outputs: output, error
	if methodType.NumOut() != 2 {
		return false
	}

	// Check first param is *Execution
	executionPtrType := reflect.TypeOf((*Execution)(nil))
	if methodType.In(1) != executionPtrType {
		return false
	}

	// Check second param is EITHER map[string]any OR struct
	inputType := methodType.In(2)
	if !isMapOrStruct(inputType) {
		return false
	}

	// Check first return is EITHER map[string]any OR struct
	outputType := methodType.Out(0)
	if !isMapOrStruct(outputType) {
		return false
	}

	// Check second return is error
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if methodType.Out(1) != errorType {
		return false
	}

	return true
}

// isMapOrStruct checks if type is either map[string]any or a struct
func isMapOrStruct(t reflect.Type) bool {
	// Check if map[string]any
	if t.Kind() == reflect.Map {
		mapType := reflect.TypeOf(map[string]any(nil))
		return t == mapType
	}

	// Check if struct
	return t.Kind() == reflect.Struct
}

// isTypedSignature checks if method uses typed (struct) signature
func isTypedSignature(methodType reflect.Type) bool {
	inputType := methodType.In(2)
	outputType := methodType.Out(0)

	// Consider it typed if either input or output is a struct
	return inputType.Kind() == reflect.Struct || outputType.Kind() == reflect.Struct
}

// toLowerFirst converts first character of string to lowercase
func toLowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// createTaskExecutor creates a Task wrapper for a plugin method
// It automatically detects if the method uses typed or map-based signature
func createTaskExecutor(pluginValue reflect.Value, method reflect.Method) Task {
	// Check if method uses typed signature
	if isTypedSignature(method.Type) {
		return &typedTaskWrapper{
			plugin:     pluginValue,
			method:     method,
			inputType:  method.Type.In(2),
			outputType: method.Type.Out(0),
		}
	}

	// Otherwise use map-based wrapper
	return &pluginTaskWrapper{
		plugin: pluginValue,
		method: method,
	}
}

// pluginTaskWrapper wraps a plugin method to implement Task interface
type pluginTaskWrapper struct {
	plugin reflect.Value
	method reflect.Method
}

func (w *pluginTaskWrapper) Execute(exec *Execution, args map[string]any) (map[string]any, error) {
	// Call plugin method using reflection
	results := w.method.Func.Call([]reflect.Value{
		w.plugin,
		reflect.ValueOf(exec),
		reflect.ValueOf(args),
	})

	// Extract result and error
	resultMap := results[0].Interface().(map[string]any)

	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	return resultMap, err
}

// typedTaskWrapper wraps a typed plugin method and handles conversion
type typedTaskWrapper struct {
	plugin     reflect.Value
	method     reflect.Method
	inputType  reflect.Type
	outputType reflect.Type
}

func (w *typedTaskWrapper) Execute(exec *Execution, args map[string]any) (map[string]any, error) {
	// Step 1: Convert map → struct
	inputPtr := reflect.New(w.inputType)
	if err := mapToStruct(args, inputPtr.Interface()); err != nil {
		slog.Error("Task input conversion failed",
			"task", w.method.Name,
			"args", args,
			"error", err)
		return nil, fmt.Errorf("invalid input for task %s: %w", w.method.Name, err)
	}

	// Step 2: Validate input struct using existing validation framework
	if err := validateConfig(inputPtr.Interface()); err != nil {
		slog.Error("Task input validation failed",
			"task", w.method.Name,
			"args", args,
			"error", err)
		return nil, fmt.Errorf("validation failed for task %s: %w", w.method.Name, err)
	}

	// Step 3: Call typed method via reflection
	results := w.method.Func.Call([]reflect.Value{
		w.plugin,
		reflect.ValueOf(exec),
		inputPtr.Elem(), // Dereference pointer to pass struct value
	})

	// Step 4: Extract output and error from method call
	output := results[0].Interface()
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}

	// Step 5: Convert struct → map
	resultMap, convertErr := structToMap(output)
	if convertErr != nil {
		slog.Error("Task output conversion failed",
			"task", w.method.Name,
			"error", convertErr)
		return nil, fmt.Errorf("failed to convert output for task %s: %w", w.method.Name, convertErr)
	}

	return resultMap, err
}

// isValidResponseHandlerSignature checks if method has valid response handler signature
// Valid signature: func(c *gin.Context, exec *Execution, args map[string]any) error
func isValidResponseHandlerSignature(methodType reflect.Type) bool {
	// Must have 4 inputs: receiver, *gin.Context, *Execution, map[string]any
	if methodType.NumIn() != 4 {
		return false
	}

	// Must have 1 output: error
	if methodType.NumOut() != 1 {
		return false
	}

	// Check first param is *gin.Context
	ginContextPtrType := reflect.TypeOf((*gin.Context)(nil))
	if methodType.In(1) != ginContextPtrType {
		return false
	}

	// Check second param is *Execution
	executionPtrType := reflect.TypeOf((*Execution)(nil))
	if methodType.In(2) != executionPtrType {
		return false
	}

	// Check third param is map[string]any
	mapType := reflect.TypeOf(map[string]any(nil))
	if methodType.In(3) != mapType {
		return false
	}

	// Check return is error
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if methodType.Out(0) != errorType {
		return false
	}

	return true
}

// pluginResponseHandlerWrapper wraps a plugin method to implement ResponseHandler interface
type pluginResponseHandlerWrapper struct {
	plugin reflect.Value
	method reflect.Method
}

func (w *pluginResponseHandlerWrapper) Handle(c *gin.Context, exec *Execution, args map[string]any) error {
	// Call plugin method using reflection
	results := w.method.Func.Call([]reflect.Value{
		w.plugin,
		reflect.ValueOf(c),
		reflect.ValueOf(exec),
		reflect.ValueOf(args),
	})

	// Extract error (only return value)
	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}

// createResponseHandlerWrapper creates a ResponseHandler wrapper for a plugin method
func createResponseHandlerWrapper(pluginValue reflect.Value, method reflect.Method) ResponseHandler {
	return &pluginResponseHandlerWrapper{
		plugin: pluginValue,
		method: method,
	}
}
