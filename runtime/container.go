package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// Interface type constants for plugin capabilities
const (
	InterfaceLifecycle = "Lifecycle"
)

type Container struct {
	Tasks              map[string]Task
	plugins            map[string]any   // Plugin instances (name -> plugin)
	pluginsByInterface map[string][]any // Interface name -> plugins implementing that interface
}

func NewContainer() *Container {
	return &Container{
		Tasks:              make(map[string]Task),
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

	// Discover and register all tasks from plugin methods
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
		if !isValidTaskSignature(method.Type) {
			continue
		}

		// Create task name: plugin_name.method_name (lowercase)
		taskName := fmt.Sprintf("%s.%s", pluginName, toLowerFirst(method.Name))

		// Create task executor wrapper
		taskExecutor := createTaskExecutor(pluginValue, method)

		// Register task
		c.Tasks[taskName] = taskExecutor
	}

	return nil
}

// detectPluginInterfaces detects which interfaces a plugin implements and registers them
func (c *Container) detectPluginInterfaces(plugin any) {
	// Check Lifecycle interface
	if _, ok := plugin.(Lifecycle); ok {
		c.pluginsByInterface[InterfaceLifecycle] = append(
			c.pluginsByInterface[InterfaceLifecycle],
			plugin,
		)
	}

	// Future: Add more interface checks here
	// if _, ok := plugin.(HealthChecker); ok {
	//     c.pluginsByInterface[InterfaceHealthChecker] = append(...)
	// }
}

// GetPlugin returns a plugin instance by name (for Phase 1 manual lookup)
func (c *Container) GetPlugin(name string) any {
	return c.plugins[name]
}

// Initialize calls Initialize on all plugins implementing Lifecycle interface
// For Phase 1 MVP: Uses fail-fast approach (panics on any error)
func (c *Container) Initialize(ctx context.Context) error {
	// Get lifecycle plugins from registry (interface check already done during registration)
	lifecyclePlugins := c.pluginsByInterface[InterfaceLifecycle]

	for i, plugin := range lifecyclePlugins {
		lifecycle := plugin.(Lifecycle)
		if err := lifecycle.Initialize(ctx); err != nil {
			// Phase 1: Fail-fast with panic
			panic(fmt.Sprintf("plugin #%d initialization failed: %v", i, err))
		}
	}
	return nil
}

// Shutdown calls Shutdown on all plugins implementing Lifecycle interface
// Plugins are shut down in reverse order of initialization
func (c *Container) Shutdown(ctx context.Context) error {
	// Get lifecycle plugins from registry
	lifecyclePlugins := c.pluginsByInterface[InterfaceLifecycle]

	// Shutdown in reverse order
	var errors []error
	for i := len(lifecyclePlugins) - 1; i >= 0; i-- {
		lifecycle := lifecyclePlugins[i].(Lifecycle)
		if err := lifecycle.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("plugin #%d shutdown failed: %w", i, err))
		}
	}

	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// isValidTaskSignature checks if method has valid task signature
// Valid: func(exec *Execution, args map[string]any) (map[string]any, error)
func isValidTaskSignature(methodType reflect.Type) bool {
	// Must have 3 inputs: receiver, *Execution, map[string]any
	if methodType.NumIn() != 3 {
		return false
	}

	// Must have 2 outputs: map[string]any, error
	if methodType.NumOut() != 2 {
		return false
	}

	// Check input types
	executionPtrType := reflect.TypeOf((*Execution)(nil))
	mapType := reflect.TypeOf(map[string]any(nil))

	if methodType.In(1) != executionPtrType {
		return false
	}

	if methodType.In(2) != mapType {
		return false
	}

	// Check output types
	errorType := reflect.TypeOf((*error)(nil)).Elem()

	if methodType.Out(0) != mapType {
		return false
	}

	if methodType.Out(1) != errorType {
		return false
	}

	return true
}

// toLowerFirst converts first character of string to lowercase
func toLowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// createTaskExecutor creates a Task wrapper for a plugin method
func createTaskExecutor(pluginValue reflect.Value, method reflect.Method) Task {
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
