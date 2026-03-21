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
	tasks            map[string]Task
	ResponseHandlers *ResponseHandlerRegistry
	plugins          *pluginRegistry
	observability    *ObservabilityRuntime
	logger           Logger
	tracer           trace.Tracer
	metrics          *Metrics
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
		tasks:            make(map[string]Task),
		ResponseHandlers: NewResponseHandlerRegistry(),
		plugins:          newPluginRegistry(),
		logger:           logger,
		tracer:           newNoopTracer(),
		metrics:          NewNoopMetrics(),
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
