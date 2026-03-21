package runtime

import (
	"context"
	"fmt"

	"github.com/BDNK1/sflowg/runtime/internal/pluginexec"
	"go.opentelemetry.io/otel/trace"
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
	pluginNameIndex    map[any]string   // Reverse lookup: plugin instance -> name
	observability      *ObservabilityRuntime
	logger             Logger
	tracer             trace.Tracer
	metrics            *Metrics
}

// Logger returns the container's logger for framework-level (non-execution) logs.
// Use execution.Logger() instead when an *Execution is available.
func (c *Container) Logger() Logger {
	return c.logger
}

func (c *Container) Tracer() trace.Tracer {
	if c.tracer == nil {
		return newNoopTracer()
	}
	return c.tracer
}

func (c *Container) Metrics() *Metrics {
	if c.metrics == nil {
		return NewNoopMetrics()
	}
	return c.metrics
}

func NewContainer(logger Logger) *Container {
	return &Container{
		Tasks:              make(map[string]Task),
		ResponseHandlers:   NewResponseHandlerRegistry(),
		plugins:            make(map[string]any),
		pluginsByInterface: make(map[string][]any),
		pluginNameIndex:    make(map[any]string),
		logger:             logger,
		tracer:             newNoopTracer(),
		metrics:            NewNoopMetrics(),
	}
}

func (c *Container) InitObservability(cfg ObservabilityConfig) error {
	obs, err := InitObservability(cfg)
	if err != nil {
		return err
	}
	c.SetObservability(obs)
	return nil
}

func (c *Container) SetObservability(obs *ObservabilityRuntime) {
	c.observability = obs
	if obs == nil {
		c.logger = NewLogger(nil)
		c.tracer = newNoopTracer()
		c.metrics = NewNoopMetrics()
		return
	}

	c.logger = obs.Logger
	if obs.Tracer == nil {
		c.tracer = newNoopTracer()
	} else {
		c.tracer = obs.Tracer
	}
	if obs.Metrics == nil {
		c.metrics = NewNoopMetrics()
	} else {
		c.metrics = obs.Metrics
	}
}

func (c *Container) ShutdownObservability(ctx context.Context) error {
	if c.observability == nil {
		return nil
	}
	err := c.observability.Shutdown(ctx)
	c.observability = nil
	c.logger = NewLogger(nil)
	c.tracer = newNoopTracer()
	c.metrics = NewNoopMetrics()
	return err
}

func (c *Container) SetTracer(tracer trace.Tracer) {
	if tracer == nil {
		c.tracer = newNoopTracer()
		return
	}
	c.tracer = tracer
}

func (c *Container) SetMetrics(metrics *Metrics) {
	if metrics == nil {
		c.metrics = NewNoopMetrics()
		return
	}
	c.metrics = metrics
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

	c.plugins[pluginName] = plugin
	c.pluginNameIndex[plugin] = pluginName
	c.detectPluginInterfaces(plugin)

	taskBindings, responseBindings := pluginexec.Discover(pluginName, plugin)
	for _, binding := range taskBindings {
		c.Tasks[binding.TaskName] = newTaskExecutor(binding)
	}
	for _, binding := range responseBindings {
		c.ResponseHandlers.Register(binding.HandlerName, newResponseHandler(binding))
	}

	return nil
}

func (c *Container) detectPluginInterfaces(plugin any) {
	if _, ok := plugin.(Initializer); ok {
		c.pluginsByInterface[InterfaceInitializer] = append(
			c.pluginsByInterface[InterfaceInitializer],
			plugin,
		)
	}

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
// Returns the first initialization error.
func (c *Container) Initialize(ctx context.Context) error {
	initializerPlugins := c.pluginsByInterface[InterfaceInitializer]

	for _, p := range initializerPlugins {
		initializer := p.(Initializer)
		name := c.pluginName(p)
		if err := initializer.Initialize(c.logger.ForPlugin(name).With("plugin", name)); err != nil {
			return fmt.Errorf("plugin %q initialization failed: %w", name, err)
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
		name := c.pluginName(shutdownerPlugins[i])
		if err := shutdowner.Shutdown(c.logger.ForPlugin(name).With("plugin", name)); err != nil {
			errors = append(errors, fmt.Errorf("plugin #%d shutdown failed: %w", i, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

func (c *Container) pluginName(plugin any) string {
	return c.pluginNameIndex[plugin]
}
