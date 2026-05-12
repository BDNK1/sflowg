package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/BDNK1/sflowg/core/internal/configutil"
	validationschema "github.com/BDNK1/sflowg/core/validation/schema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	flowInputNamespace = "input"
	maxSubflowDepth    = 50
)

type subflowDepthKey struct{}

func subflowDepth(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(subflowDepthKey{}).(int)
	return v
}

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
	flowValidator    FlowSetValidator
	runtimeConfig    RuntimeConfig
}

type preparedFlows struct {
	flows            map[string]Flow
	activeTransports []Transport
}

// NewApp creates a new application with the given container and engine components.
// Passing a non-nil compiler enables compiled DSL mode; nil keeps interpreted DSL mode.
// The container must be initialized with a logger before calling NewApp.
func NewApp(container *Container, loader FlowLoader, evaluator ExpressionEvaluator, stepExecutor StepExecutor, stepRunner StepRunner, compiler FlowCompiler, newValueStore func() ValueStore, runtimeConfigs ...RuntimeConfig) *App {
	runtimeConfig := RuntimeConfig{}
	if len(runtimeConfigs) > 0 {
		runtimeConfig = runtimeConfigs[0]
	}
	container.SetRuntimeConfig(runtimeConfig)
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
		runtimeConfig:    runtimeConfig,
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

func (a *App) SetFlowValidator(validator FlowSetValidator) {
	a.flowValidator = validator
}

func (a *App) Executor() *Executor {
	return NewExecutor(a.evaluator, a.stepExecutor, a.stepRunner)
}

// Start starts registered transports and blocks until shutdown.
func (a *App) Start(ctx context.Context, flowsDir string) error {
	// Initialize plugins
	if err := a.initialize(ctx); err != nil {
		return err
	}
	cleanupStartupError := func(startupErr error) error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if shutdownErr := a.Container.Shutdown(shutdownCtx); shutdownErr != nil {
			return fmt.Errorf("%w; shutdown: %v", startupErr, shutdownErr)
		}
		return startupErr
	}

	// Wire metric context from properties.observability.metrics.context.
	if err := a.applyMetricContext(); err != nil {
		return cleanupStartupError(err)
	}

	a.Container.Logger().Info("DSL execution mode", "mode", a.dslMode())
	prepared, err := a.prepareFlows(ctx, flowsDir)
	if err != nil {
		return cleanupStartupError(err)
	}
	a.Flows = prepared.flows
	activeTransports := prepared.activeTransports
	if len(activeTransports) == 0 {
		return cleanupStartupError(fmt.Errorf("no transports to start"))
	}
	grouped := groupFlowsByTransport(a.Flows, activeTransports)

	// Create executor for flow execution
	executor := a.Executor()

	errChan := make(chan error, len(activeTransports))

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
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shutdownErr := a.shutdown(shutdownCtx, activeTransports)
		drained := drainTransportErrors(errChan, len(activeTransports))
		return errors.Join(append([]error{ctx.Err(), shutdownErr}, drained...)...)
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shutdownErr := a.shutdown(shutdownCtx, activeTransports)
		drained := drainTransportErrors(errChan, len(activeTransports)-1)
		return errors.Join(append([]error{err, shutdownErr}, drained...)...)
	}
}

func drainTransportErrors(ch <-chan error, remaining int) []error {
	if remaining <= 0 {
		return nil
	}
	out := make([]error, 0, remaining)
	for i := 0; i < remaining; i++ {
		select {
		case err := <-ch:
			if err != nil {
				out = append(out, err)
			}
		default:
			return out
		}
	}
	return out
}

func (a *App) prepareFlows(ctx context.Context, flowsDir string) (preparedFlows, error) {
	loaded, err := a.loadFlowMap(flowsDir)
	if err != nil {
		return preparedFlows{}, err
	}

	annotated, activeTransports, err := a.annotateFlowsByTransport(loaded)
	if err != nil {
		return preparedFlows{}, err
	}

	if a.flowValidator != nil {
		if err := a.flowValidator.ValidateFlows(annotated); err != nil {
			return preparedFlows{}, fmt.Errorf("validating flows: %w", err)
		}
	}

	prepared := annotated
	if a.compiler != nil {
		prepared, err = a.compileFlowMap(ctx, annotated)
		if err != nil {
			return preparedFlows{}, err
		}
	}

	return preparedFlows{
		flows:            prepared,
		activeTransports: activeTransports,
	}, nil
}

// loadFlowMap loads flow definitions from the specified directory using the configured FlowLoader.
func (a *App) loadFlowMap(flowsDir string) (map[string]Flow, error) {
	if flowsDir == "" {
		return nil, fmt.Errorf("flows directory not specified")
	}

	// Collect files matching all loader extensions
	var files []string
	for _, ext := range a.loader.Extensions() {
		matched, err := filepath.Glob(filepath.Join(flowsDir, ext))
		if err != nil {
			return nil, fmt.Errorf("error reading flows directory: %w", err)
		}
		files = append(files, matched...)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no flow files found in %s", flowsDir)
	}

	flows := make(map[string]Flow, len(files))
	for _, file := range files {
		flow, err := a.loader.Load(file)
		if err != nil {
			return nil, fmt.Errorf("error loading flow from %s: %w", file, err)
		}
		resolvedProps, err := configutil.ResolvePropertyMap(flow.Properties)
		if err != nil {
			return nil, fmt.Errorf("error resolving properties for flow %s: %w", flow.ID, err)
		}
		flow.Properties = resolvedProps
		flow.DSLMode = a.dslMode()
		flows[flow.ID] = flow
	}

	return flows, nil
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

func (a *App) annotateFlowsByTransport(flows map[string]Flow) (map[string]Flow, []Transport, error) {
	annotated := make(map[string]Flow, len(flows))
	grouped := make(map[string][]Flow)
	for flowID, flow := range flows {
		transportType := flow.Entrypoint.Type
		transport, ok := a.transports.Get(transportType)
		if !ok {
			return nil, nil, fmt.Errorf("missing transport for entrypoint %q in flow %q", transportType, flow.ID)
		}
		flow, err := annotateFlowWithEntrypointSpec(flow, transport)
		if err != nil {
			return nil, nil, err
		}
		annotated[flowID] = flow
		grouped[transportType] = append(grouped[transportType], flow)
	}

	var active []Transport
	for _, transport := range a.transports.order {
		if len(grouped[transport.Type()]) > 0 {
			if validator, ok := transport.(TransportFlowSetValidator); ok {
				if err := validator.ValidateTransportFlows(grouped[transport.Type()]); err != nil {
					return nil, nil, fmt.Errorf("validating %s flow set: %w", transport.Type(), err)
				}
			}
			active = append(active, transport)
		}
	}
	return annotated, active, nil
}

func annotateFlowWithEntrypointSpec(flow Flow, spec EntrypointSpec) (Flow, error) {
	if err := spec.ValidateFlow(flow); err != nil {
		return Flow{}, fmt.Errorf("validating %s entrypoint for flow %q: %w", spec.Type(), flow.ID, err)
	}
	flow.ResponseContract = spec.ResponseContract()
	flow.ResponseSubtypes = flow.ResponseContract.Subtypes()
	return flow, nil
}

func groupFlowsByTransport(flows map[string]Flow, activeTransports []Transport) map[string][]Flow {
	grouped := make(map[string][]Flow, len(activeTransports))
	activeTypes := make(map[string]struct{}, len(activeTransports))
	for _, transport := range activeTransports {
		activeTypes[transport.Type()] = struct{}{}
	}
	for _, flow := range flows {
		if _, ok := activeTypes[flow.Entrypoint.Type]; !ok {
			continue
		}
		grouped[flow.Entrypoint.Type] = append(grouped[flow.Entrypoint.Type], flow)
	}
	return grouped
}

func (a *App) dslMode() DSLExecutionMode {
	if a.compiler != nil {
		return DSLExecutionModeCompiled
	}
	return DSLExecutionModeInterpreted
}

func (a *App) compileFlowMap(ctx context.Context, flows map[string]Flow) (map[string]Flow, error) {
	compiled := make(map[string]Flow, len(flows))
	for flowID, flow := range flows {
		if err := a.compiler.CompileFlow(ctx, &flow, a.Container); err != nil {
			return nil, fmt.Errorf("compiling flow %s: %w", flowID, err)
		}
		compiled[flowID] = flow
	}
	return compiled, nil
}

func (a *App) LookupFlow(name string) (*Flow, bool) {
	flow, ok := a.Flows[name]
	if !ok {
		return nil, false
	}
	return &flow, true
}

func (a *App) InvokeSubflow(parent *Execution, target *Flow, args map[string]any) (map[string]any, error) {
	if target == nil {
		return nil, &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: "subflow target is nil"}
	}

	depth := subflowDepth(parent) + 1
	if depth > maxSubflowDepth {
		return nil, &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeSubflowDepth),
			Message: fmt.Sprintf("subflow %q exceeds maximum recursion depth of %d", target.ID, maxSubflowDepth),
		}
	}

	normalized, err := validateSubflowArgs(target, args)
	if err != nil {
		return nil, err
	}

	sub := NewExecution(target, parent.Container, a.GlobalProperties, a.newValueStore())
	sub = sub.WithContext(parent)
	for key, value := range normalized {
		sub.AddValue(flowInputNamespace+"."+key, value)
	}

	ctx, span := parent.Tracer().Start(parent, fmt.Sprintf("flow.call %s", target.ID),
		trace.WithAttributes(attribute.String("flow.call.target", target.ID)),
	)
	defer span.End()
	subCtx := context.WithValue(ctx, subflowDepthKey{}, depth)
	cancel := func() {}
	if target.Timeout > 0 {
		subCtx, cancel = context.WithTimeout(subCtx, time.Duration(target.Timeout)*time.Millisecond)
	}
	defer cancel()
	sub = sub.WithContext(subCtx)

	if err := a.Executor().ExecuteSteps(sub); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	rd := sub.State().Response()
	if rd == nil {
		err := &FlowError{Type: ErrorTypePermanent, Code: "SUBFLOW_NO_RESPONSE", Message: fmt.Sprintf("subflow %q completed without response.value or response.error", target.ID)}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Message)
		return nil, err
	}
	switch rd.Subtype {
	case "value":
		return rd.Args, nil
	case "error":
		fe := flowResponseError(rd.Args)
		span.RecordError(fe)
		span.SetStatus(codes.Error, fe.Message)
		return nil, fe
	default:
		err := &FlowError{Type: ErrorTypePermanent, Code: string(ErrorCodeRuntimeError), Message: fmt.Sprintf("subflow %q returned unsupported response.%s", target.ID, rd.Subtype)}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Message)
		return nil, err
	}
}

func validateSubflowArgs(target *Flow, args map[string]any) (map[string]any, error) {
	if args == nil {
		args = map[string]any{}
	}
	fields := target.Entrypoint.Input.FieldSchemas(flowInputNamespace)
	normalized := make(map[string]any, len(args)+len(fields))
	for key, value := range args {
		normalized[key] = value
	}
	var fieldErrs []validationschema.FieldError
	for name, fieldSchema := range fields {
		raw, present := args[name]
		value, errs := validationschema.ValidateField(raw, present, fieldSchema, flowInputNamespace+"."+name)
		if len(errs) > 0 {
			fieldErrs = append(fieldErrs, errs...)
			continue
		}
		if value != nil || present || fieldSchema.Default != nil {
			normalized[name] = value
		}
	}
	if len(fieldErrs) > 0 {
		return nil, &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeSchemaViolation),
			Message: fmt.Sprintf("subflow %q input validation failed", target.ID),
			Meta: map[string]any{
				"fields": validationschema.FieldsToMaps(fieldErrs),
			},
		}
	}
	return normalized, nil
}

func flowResponseError(args map[string]any) *FlowError {
	code, _ := args["code"].(string)
	message, _ := args["message"].(string)
	if code == "" {
		code = string(ErrorCodeRuntimeError)
	}
	if message == "" {
		message = code
	}
	return &FlowError{Type: ErrorTypePermanent, Code: code, Message: message}
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
