package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/BDNK1/sflowg/core/internal/pluginexec"
)

type pluginRegistry struct {
	mu                 sync.RWMutex
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

func (r *pluginRegistry) Register(pluginName string, plugin any) ([]pluginexec.TaskBinding, []pluginexec.SignatureIssue, error) {
	if plugin == nil {
		return nil, nil, fmt.Errorf("plugin cannot be nil")
	}

	r.mu.Lock()
	r.plugins[pluginName] = plugin
	r.pluginNameIndex[plugin] = pluginName
	r.detectPluginInterfacesLocked(plugin)
	r.mu.Unlock()

	taskBindings, issues := pluginexec.Discover(pluginName, plugin)
	return taskBindings, issues, nil
}

func (r *pluginRegistry) Get(name string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[name]
}

func (r *pluginRegistry) snapshotByInterface(name string) ([]any, []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugins := r.pluginsByInterface[name]
	out := make([]any, len(plugins))
	names := make([]string, len(plugins))
	for i, p := range plugins {
		out[i] = p
		names[i] = r.pluginNameIndex[p]
	}
	return out, names
}

func (r *pluginRegistry) Initialize(ctx context.Context, logger Logger) error {
	initializers, names := r.snapshotByInterface(InterfaceInitializer)
	for i, p := range initializers {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("plugin initialization cancelled: %w", err)
		}
		initializer := p.(Initializer)
		name := names[i]
		done := make(chan error, 1)
		go func() {
			done <- initializer.Initialize(logger.ForPlugin(name).With("plugin", name))
		}()
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("plugin %q initialization failed: %w", name, err)
			}
		case <-ctx.Done():
			return fmt.Errorf("plugin %q initialization cancelled: %w", name, ctx.Err())
		}
	}
	return nil
}

func (r *pluginRegistry) Shutdown(ctx context.Context, logger Logger) error {
	shutdowners, names := r.snapshotByInterface(InterfaceShutdowner)
	var errs []error
	for i := len(shutdowners) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("plugin shutdown cancelled: %w", err))
			break
		}
		shutdowner := shutdowners[i].(Shutdowner)
		name := names[i]
		done := make(chan error, 1)
		go func() {
			done <- shutdowner.Shutdown(logger.ForPlugin(name).With("plugin", name))
		}()
		select {
		case err := <-done:
			if err != nil {
				errs = append(errs, fmt.Errorf("plugin %q shutdown failed: %w", name, err))
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("plugin %q shutdown cancelled: %w", name, ctx.Err()))
		}
	}
	return errors.Join(errs...)
}

func (r *pluginRegistry) detectPluginInterfacesLocked(plugin any) {
	if _, ok := plugin.(Initializer); ok {
		r.pluginsByInterface[InterfaceInitializer] = append(r.pluginsByInterface[InterfaceInitializer], plugin)
	}

	if _, ok := plugin.(Shutdowner); ok {
		r.pluginsByInterface[InterfaceShutdowner] = append(r.pluginsByInterface[InterfaceShutdowner], plugin)
	}
}

func (c *Container) RegisterPlugin(pluginName string, plugin any) error {
	taskBindings, issues, err := c.plugins.Register(pluginName, plugin)
	if err != nil {
		return err
	}

	logger := c.Logger()
	for _, issue := range issues {
		logger.Warn("plugin method has invalid signature; skipping",
			"plugin", pluginName,
			"method", issue.MethodName,
			"reason", issue.Reason)
	}

	for _, binding := range taskBindings {
		c.registerTask(binding.TaskName, newTaskExecutor(binding))
	}

	return nil
}

func (c *Container) GetPlugin(name string) any {
	return c.plugins.Get(name)
}
