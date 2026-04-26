package runtime

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BDNK1/sflowg/runtime/internal/configutil"
)

type App struct {
	Container        *Container
	Flows            map[string]Flow
	GlobalProperties map[string]any // Global properties from flow-config.yaml
	loader           FlowLoader
	evaluator        ExpressionEvaluator
	stepExecutor     StepExecutor
	stepRunner       StepRunner
	compiler         FlowCompiler
	newValueStore    func() ValueStore
	transports       *TransportRegistry
}

// NewApp creates a new application with the given container and engine components.
// Passing a non-nil compiler enables compiled DSL mode; nil keeps interpreted DSL mode.
// The container must be initialized with a logger before calling NewApp.
func NewApp(container *Container, loader FlowLoader, evaluator ExpressionEvaluator, stepExecutor StepExecutor, stepRunner StepRunner, compiler FlowCompiler, newValueStore func() ValueStore, _ ...ObservabilityConfig) *App {
	return &App{
		Container:        container,
		Flows:            make(map[string]Flow),
		GlobalProperties: make(map[string]any),
		loader:           loader,
		evaluator:        evaluator,
		stepExecutor:     stepExecutor,
		stepRunner:       stepRunner,
		compiler:         compiler,
		newValueStore:    newValueStore,
		transports:       NewTransportRegistry(),
	}
}

// RegisterTransport registers a protocol transport used by flow entrypoints.
func (a *App) RegisterTransport(transport Transport) error {
	return a.transports.Register(transport)
}

// SetGlobalProperties sets global properties that will be merged with flow properties.
// Flow properties override global properties.
func (a *App) SetGlobalProperties(props map[string]any) error {
	resolved, err := configutil.ResolvePropertyMap(props)
	if err != nil {
		return fmt.Errorf("invalid global properties: %w", err)
	}
	a.GlobalProperties = resolved
	return nil
}

// Start starts registered transports and blocks until shutdown.
func (a *App) Start(ctx context.Context, flowsDir string) error {
	// Initialize plugins
	if err := a.initialize(ctx); err != nil {
		return err
	}

	// Wire metric context from properties.observability.metrics.context.
	if err := a.applyMetricContext(); err != nil {
		return err
	}

	// Load flows at startup (runtime resolution)
	if err := a.loadFlows(flowsDir); err != nil {
		return err
	}
	a.Container.Logger().Info("DSL execution mode", "mode", a.dslMode())
	grouped, activeTransports, err := a.groupFlowsByTransport()
	if err != nil {
		return err
	}
	if len(activeTransports) == 0 {
		return fmt.Errorf("no transports to start")
	}
	if a.compiler != nil {
		if err := a.compileFlows(ctx); err != nil {
			return err
		}
	}

	// Create executor for flow execution
	executor := NewExecutor(a.evaluator, a.stepExecutor, a.stepRunner)

	// Setup graceful shutdown
	errChan := make(chan error, len(activeTransports))
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	for _, transport := range activeTransports {
		transport := transport
		transportRuntime := TransportRuntime{
			Container:        a.Container,
			Executor:         executor,
			Flows:            grouped[transport.Type()],
			GlobalProperties: a.GlobalProperties,
			NewValueStore:    a.newValueStore,
		}
		go func() {
			if err := transport.Start(ctx, transportRuntime); err != nil {
				errChan <- fmt.Errorf("%s transport: %w", transport.Type(), err)
				return
			}
			errChan <- nil
		}()
	}

	a.Container.Logger().Info("Flows loaded", "count", len(a.Flows))

	select {
	case <-sigChan:
		a.Container.Logger().Info("Shutting down gracefully")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return a.shutdown(shutdownCtx, activeTransports)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := a.shutdown(shutdownCtx, activeTransports); err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if shutdownErr := a.shutdown(shutdownCtx, activeTransports); shutdownErr != nil {
			if err != nil {
				return fmt.Errorf("%w; shutdown: %v", err, shutdownErr)
			}
			return shutdownErr
		}
		if err != nil {
			return err
		}
		return nil
	}
}

// loadFlows loads flow definitions from the specified directory using the configured FlowLoader.
func (a *App) loadFlows(flowsDir string) error {
	if flowsDir == "" {
		return fmt.Errorf("flows directory not specified")
	}

	// Collect files matching all loader extensions
	var files []string
	for _, ext := range a.loader.Extensions() {
		matched, err := filepath.Glob(filepath.Join(flowsDir, ext))
		if err != nil {
			return fmt.Errorf("error reading flows directory: %w", err)
		}
		files = append(files, matched...)
	}

	if len(files) == 0 {
		return fmt.Errorf("no flow files found in %s", flowsDir)
	}

	for _, file := range files {
		flow, err := a.loader.Load(file)
		if err != nil {
			return fmt.Errorf("error loading flow from %s: %w", file, err)
		}
		resolvedProps, err := configutil.ResolvePropertyMap(flow.Properties)
		if err != nil {
			return fmt.Errorf("error resolving properties for flow %s: %w", flow.ID, err)
		}
		flow.Properties = resolvedProps
		a.registerFlow(flow)
	}

	return nil
}

// Initialize initializes the container (calls plugin Initialize methods).
// Must be called after plugins are registered and before LoadFlows.
func (a *App) initialize(ctx context.Context) error {
	if err := a.Container.Initialize(ctx); err != nil {
		return fmt.Errorf("container initialization failed: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down transports and the container.
// Calls plugin Shutdown methods in reverse order of initialization.
func (a *App) shutdown(ctx context.Context, activeTransports []Transport) error {
	var errors []error

	for i := len(activeTransports) - 1; i >= 0; i-- {
		transport := activeTransports[i]
		a.Container.Logger().Info("Shutting down transport", "type", transport.Type())
		if err := transport.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("%s transport shutdown: %w", transport.Type(), err))
		}
	}

	// Shutdown container (calls plugin Shutdown methods)
	a.Container.Logger().Info("Shutting down plugins")
	if err := a.Container.Shutdown(ctx); err != nil {
		errors = append(errors, fmt.Errorf("container shutdown: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}
	return nil
}

func (a *App) groupFlowsByTransport() (map[string][]Flow, []Transport, error) {
	grouped := make(map[string][]Flow)
	for flowID := range a.Flows {
		flow := a.Flows[flowID]
		transportType := flow.Entrypoint.Type
		transport, ok := a.transports.Get(transportType)
		if !ok {
			return nil, nil, fmt.Errorf("missing transport for entrypoint %q in flow %q", transportType, flow.ID)
		}
		if err := transport.ValidateFlow(flow); err != nil {
			return nil, nil, fmt.Errorf("validating %s entrypoint for flow %q: %w", transportType, flow.ID, err)
		}
		flow.ResponseSubtypes = transport.ResponseSubtypes()
		a.Flows[flowID] = flow
		grouped[transportType] = append(grouped[transportType], flow)
	}

	var active []Transport
	for _, transport := range a.transports.order {
		if len(grouped[transport.Type()]) > 0 {
			active = append(active, transport)
		}
	}
	return grouped, active, nil
}

func (a *App) registerFlow(flow Flow) {
	flow.DSLMode = a.dslMode()
	a.Flows[flow.ID] = flow
}

func (a *App) dslMode() DSLExecutionMode {
	if a.compiler != nil {
		return DSLExecutionModeCompiled
	}
	return DSLExecutionModeInterpreted
}

func (a *App) compileFlows(ctx context.Context) error {
	for flowID := range a.Flows {
		flow := a.Flows[flowID]
		if err := a.compiler.CompileFlow(ctx, &flow, a.Container); err != nil {
			return fmt.Errorf("compiling flow %s: %w", flowID, err)
		}
		a.Flows[flowID] = flow
	}
	return nil
}

// applyMetricContext reads properties.observability.metrics.context from
// GlobalProperties and wires it into the metrics singleton so those labels
// are auto-attached to all user-defined DSL metrics.
func (a *App) applyMetricContext() error {
	raw, err := extractNestedMap(a.GlobalProperties, "observability", "metrics", "context")
	if err != nil {
		return fmt.Errorf("properties.observability.metrics.context: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := ValidateUserMetricContext(raw); err != nil {
		return err
	}
	a.Container.Metrics().SetUserMetricContext(ResolveUserMetricContext(raw))
	return nil
}

// extractNestedMap navigates a chain of string keys through nested map[string]any values.
// Returns nil, nil if any key is absent. Returns an error if a key exists but its value
// is not a map[string]any.
func extractNestedMap(m map[string]any, keys ...string) (map[string]any, error) {
	current := m
	for _, key := range keys {
		val, ok := current[key]
		if !ok {
			return nil, nil
		}
		next, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%q must be a map", key)
		}
		current = next
	}
	return current, nil
}
