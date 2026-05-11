package core

import (
	"sync"

	"go.opentelemetry.io/otel/trace"
)

const (
	InterfaceInitializer = "Initializer"
	InterfaceShutdowner  = "Shutdowner"
)

type Container struct {
	mu      sync.RWMutex
	tasks   map[string]Task
	plugins *pluginRegistry
	logger  Logger
	tracer  trace.Tracer
	metrics *Metrics
}

func (c *Container) Logger() Logger {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.logger
}

func (c *Container) Tracer() trace.Tracer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tracer == nil {
		return newNoopTracer()
	}
	return c.tracer
}

func (c *Container) Metrics() *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.metrics == nil {
		return NewNoopMetrics()
	}
	return c.metrics
}

func NewContainer(logger Logger) *Container {
	return &Container{
		tasks:   make(map[string]Task),
		plugins: newPluginRegistry(),
		logger:  logger,
		tracer:  newNoopTracer(),
		metrics: NewNoopMetrics(),
	}
}

func (c *Container) SetObservabilityServices(logger Logger, tracer trace.Tracer, metrics *Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if tracer == nil {
		c.tracer = newNoopTracer()
		return
	}
	c.tracer = tracer
}

func (c *Container) SetMetrics(metrics *Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if metrics == nil {
		c.metrics = NewNoopMetrics()
		return
	}
	c.metrics = metrics
}
