package runtime

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

const (
	InterfaceInitializer = "Initializer"
	InterfaceShutdowner  = "Shutdowner"
)

type Container struct {
	mu           sync.RWMutex
	tasks        map[string]Task
	plugins      *pluginRegistry
	logger       Logger
	tracer       trace.Tracer
	metrics      *Metrics
	asyncRuntime *AsyncRuntime
	runtimeCfg   RuntimeConfig
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
		tasks:        make(map[string]Task),
		plugins:      newPluginRegistry(),
		logger:       logger,
		tracer:       newNoopTracer(),
		metrics:      NewNoopMetrics(),
		asyncRuntime: NewAsyncRuntime(AsyncConfig{}),
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

func (c *Container) AsyncRuntime() *AsyncRuntime {
	c.mu.RLock()
	rt := c.asyncRuntime
	c.mu.RUnlock()
	if rt != nil {
		return rt
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.asyncRuntime == nil {
		c.asyncRuntime = NewAsyncRuntime(AsyncConfig{})
	}
	return c.asyncRuntime
}

func (c *Container) SetAsyncRuntime(runtime *AsyncRuntime) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if runtime == nil {
		runtime = NewAsyncRuntime(AsyncConfig{})
	}
	c.asyncRuntime = runtime
}

func (c *Container) RuntimeConfig() RuntimeConfig {
	if c == nil {
		return RuntimeConfig{Parallel: NormalizeParallelConfig(ParallelConfig{})}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg := c.runtimeCfg
	cfg.Parallel = NormalizeParallelConfig(cfg.Parallel)
	return cfg
}

func (c *Container) SetRuntimeConfig(cfg RuntimeConfig) {
	c.mu.Lock()
	c.runtimeCfg = cfg
	c.runtimeCfg.Parallel = NormalizeParallelConfig(c.runtimeCfg.Parallel)
	c.mu.Unlock()
	c.SetAsyncRuntime(NewAsyncRuntime(cfg.Async))
}

func (c *Container) Initialize(ctx context.Context) error {
	return c.plugins.Initialize(ctx, c.Logger())
}

func (c *Container) Shutdown(ctx context.Context) error {
	c.mu.RLock()
	rt := c.asyncRuntime
	c.mu.RUnlock()
	if rt != nil {
		rt.Shutdown()
	}
	return c.plugins.Shutdown(ctx, c.Logger())
}
