package runtime

import (
	"context"
	"fmt"

	"github.com/BDNK1/sflowg/runtime/internal/pluginexec"
)

type pluginRegistry struct {
	plugins            map[string]any
	pluginsByInterface map[string][]any
	pluginNameIndex    map[any]string
}

func newPluginRegistry() *pluginRegistry {
	return &pluginRegistry{
		plugins:            make(map[string]any),
		pluginsByInterface: make(map[string][]any),
		pluginNameIndex:    make(map[any]string),
	}
}

func (r *pluginRegistry) Register(pluginName string, plugin any) ([]pluginexec.TaskBinding, []pluginexec.ResponseBinding, error) {
	if plugin == nil {
		return nil, nil, fmt.Errorf("plugin cannot be nil")
	}

	r.plugins[pluginName] = plugin
	r.pluginNameIndex[plugin] = pluginName
	r.detectPluginInterfaces(plugin)

	taskBindings, responseBindings := pluginexec.Discover(pluginName, plugin)
	return taskBindings, responseBindings, nil
}

func (r *pluginRegistry) Get(name string) any {
	return r.plugins[name]
}

func (r *pluginRegistry) Initialize(ctx context.Context, logger Logger) error {
	_ = ctx

	initializerPlugins := r.pluginsByInterface[InterfaceInitializer]
	for _, p := range initializerPlugins {
		initializer := p.(Initializer)
		name := r.pluginName(p)
		if err := initializer.Initialize(logger.ForPlugin(name).With("plugin", name)); err != nil {
			return fmt.Errorf("plugin %q initialization failed: %w", name, err)
		}
	}
	return nil
}

func (r *pluginRegistry) Shutdown(ctx context.Context, logger Logger) error {
	_ = ctx

	shutdownerPlugins := r.pluginsByInterface[InterfaceShutdowner]
	var errors []error
	for i := len(shutdownerPlugins) - 1; i >= 0; i-- {
		shutdowner := shutdownerPlugins[i].(Shutdowner)
		name := r.pluginName(shutdownerPlugins[i])
		if err := shutdowner.Shutdown(logger.ForPlugin(name).With("plugin", name)); err != nil {
			errors = append(errors, fmt.Errorf("plugin #%d shutdown failed: %w", i, err))
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

func (r *pluginRegistry) detectPluginInterfaces(plugin any) {
	if _, ok := plugin.(Initializer); ok {
		r.pluginsByInterface[InterfaceInitializer] = append(r.pluginsByInterface[InterfaceInitializer], plugin)
	}

	if _, ok := plugin.(Shutdowner); ok {
		r.pluginsByInterface[InterfaceShutdowner] = append(r.pluginsByInterface[InterfaceShutdowner], plugin)
	}
}

func (r *pluginRegistry) pluginName(plugin any) string {
	return r.pluginNameIndex[plugin]
}

func (c *Container) RegisterPlugin(pluginName string, plugin any) error {
	taskBindings, responseBindings, err := c.plugins.Register(pluginName, plugin)
	if err != nil {
		return err
	}

	for _, binding := range taskBindings {
		c.registerTask(binding.TaskName, newTaskExecutor(binding))
	}
	for _, binding := range responseBindings {
		c.ResponseHandlers.Register(binding.HandlerName, newResponseHandler(binding))
	}

	return nil
}

func (c *Container) GetPlugin(name string) any {
	return c.plugins.Get(name)
}

func (c *Container) Initialize(ctx context.Context) error {
	return c.plugins.Initialize(ctx, c.logger)
}

func (c *Container) Shutdown(ctx context.Context) error {
	return c.plugins.Shutdown(ctx, c.logger)
}
