package runtime

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Interface type constants for plugin capabilities
const (
	InterfaceInitializer = "Initializer"
	InterfaceShutdowner  = "Shutdowner"
)

type Container struct {
	tasks        map[string]Task
	plugins      *pluginRegistry
	logger       Logger
	tracer       trace.Tracer
	metrics      *Metrics
	asyncRuntime *AsyncRuntime
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
		tasks:        make(map[string]Task),
		plugins:      newPluginRegistry(),
		logger:       logger,
		tracer:       newNoopTracer(),
		metrics:      NewNoopMetrics(),
		asyncRuntime: NewAsyncRuntime(AsyncConfig{}),
	}
}

func (c *Container) SetObservabilityServices(logger Logger, tracer trace.Tracer, metrics *Metrics) {
	c.logger = logger
	if c.logger.base == nil {
		c.logger = NewLogger(nil)
	}
	if tracer == nil {
		c.tracer = newNoopTracer()
	} else {
		c.tracer = tracer
	}
	if metrics == nil {
		c.metrics = NewNoopMetrics()
	} else {
		c.metrics = metrics
	}
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

func (c *Container) AsyncRuntime() *AsyncRuntime {
	if c.asyncRuntime == nil {
		c.asyncRuntime = NewAsyncRuntime(AsyncConfig{})
	}
	return c.asyncRuntime
}

func (c *Container) SetAsyncRuntime(runtime *AsyncRuntime) {
	if runtime == nil {
		runtime = NewAsyncRuntime(AsyncConfig{})
	}
	c.asyncRuntime = runtime
}

func (c *Container) Initialize(ctx context.Context) error {
	return c.plugins.Initialize(ctx, c.logger)
}

func (c *Container) Shutdown(ctx context.Context) error {
	if c.asyncRuntime != nil {
		c.asyncRuntime.Shutdown()
	}
	return c.plugins.Shutdown(ctx, c.logger)
}
