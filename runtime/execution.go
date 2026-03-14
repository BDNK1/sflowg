package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var _ context.Context = &Execution{}

// ResponseDescriptor captures a response set by steps (YAML return or DSL response.*() calls).
// The HTTP handler dispatches them to the appropriate ResponseHandler.
type ResponseDescriptor struct {
	HandlerName string         // e.g. "http.json"
	Args        map[string]any // e.g. {status: 404, body: {...}}
}

// SuccessPath identifies which execution path (primary or fallback) succeeded for a step.
type SuccessPath string

const (
	SuccessPathPrimary  SuccessPath = "primary"
	SuccessPathFallback SuccessPath = "fallback"
)

// CompensationEntry records a step that produced side-effects and must be undone
// if a later step fails. The Path field indicates which branch succeeded so
// compensation logic can apply the correct undo operation.
type CompensationEntry struct {
	StepID string
	Body   string
	Path   SuccessPath
}

type Execution struct {
	ID                 string
	Store              ValueStore
	Flow               *Flow
	Container          *Container
	ResponseDescriptor *ResponseDescriptor
	CompensationStack  []CompensationEntry
	activeStepID       string
	activePlugin       string
	ctx                context.Context // real context carrying deadline/cancellation
}

// context.Context implementation — delegates to the embedded ctx so that real
// timeouts and cancellations propagate through all slog, Risor, and retry calls.

func (e *Execution) Deadline() (deadline time.Time, ok bool) {
	return e.ctx.Deadline()
}

func (e *Execution) Done() <-chan struct{} {
	return e.ctx.Done()
}

func (e *Execution) Err() error {
	return e.ctx.Err()
}

func (e *Execution) Value(key any) any {
	if e.ctx == nil {
		return nil
	}

	k, ok := key.(string)
	if !ok || e.Store == nil {
		return e.ctx.Value(key)
	}

	if v, found := e.Store.Get(k); found {
		return v
	}
	return e.ctx.Value(key)
}

// WithContext returns a shallow copy of the Execution with a new embedded
// context. Use this to apply a per-step timeout without mutating the parent.
// Mirrors the http.Request.WithContext pattern.
func (e *Execution) WithContext(ctx context.Context) *Execution {
	copy := *e
	copy.ctx = ctx
	return &copy
}

// WithScopedContext temporarily swaps the execution context while fn runs.
// This keeps execution state on a single object while allowing step-scoped
// deadlines/cancellation to propagate to plugins that use exec as context.Context.
// Execution is single-threaded, so temporary ctx mutation is safe here.
func (e *Execution) WithScopedContext(ctx context.Context, fn func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	prev := e.ctx
	e.ctx = ctx
	defer func() {
		e.ctx = prev
	}()
	fn()
}

func (e *Execution) AddValue(k string, v any) {
	e.Store.Set(k, v)
}

func (e *Execution) Logger() Logger {
	if e.Container == nil {
		return NewLogger(nil).WithContext(e)
	}
	base := e.Container.Logger()
	if e.activePlugin != "" {
		base = base.ForPlugin(e.activePlugin)
	}
	return base.WithContext(e)
}

func (e *Execution) WithActiveStep(stepID string, fn func()) {
	prev := e.activeStepID
	e.activeStepID = stepID
	defer func() {
		e.activeStepID = prev
	}()
	fn()
}

func (e *Execution) WithActivePlugin(pluginName string, fn func()) {
	prev := e.activePlugin
	e.activePlugin = pluginName
	defer func() {
		e.activePlugin = prev
	}()
	fn()
}

// Values returns the full context map for expression evaluation.
func (e *Execution) Values() map[string]any {
	return e.Store.All()
}

func (e *Execution) observabilityAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("execution_id", e.ID),
	}
	if e.Flow != nil && e.Flow.ID != "" {
		attrs = append(attrs, slog.String("flow_id", e.Flow.ID))
	}
	if e.activeStepID != "" {
		attrs = append(attrs, slog.String("step_id", e.activeStepID))
	}
	if e.activePlugin != "" {
		attrs = append(attrs, slog.String("plugin", e.activePlugin))
	}
	return attrs
}

func NewExecution(flow *Flow, container *Container, globalProperties map[string]any, store ValueStore) Execution {
	id := uuid.New().String()
	exec := Execution{
		ID:        id,
		Store:     store,
		Flow:      flow,
		Container: container,
		ctx:       context.Background(),
	}

	// Merge properties: global properties first, then flow properties (flow overrides).
	// Properties are resolved during startup, not per request.
	for k, v := range globalProperties {
		exec.AddValue("properties."+k, v)
	}

	for k, v := range flow.Properties {
		exec.AddValue("properties."+k, v)
	}

	return exec
}

func resolvePropertyMap(props map[string]any) (map[string]any, error) {
	if len(props) == 0 {
		return map[string]any{}, nil
	}

	resolved := make(map[string]any, len(props))
	for key, value := range props {
		resolvedValue, err := resolveEnvVar(value)
		if err != nil {
			return nil, fmt.Errorf("property %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// envVarPattern matches ${VAR} and ${VAR:default} syntax
var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)(:[^}]*)?\}$`)

// resolveEnvVar resolves environment variables in property values
func resolveEnvVar(value any) (any, error) {
	strValue, ok := value.(string)
	if !ok {
		return value, nil
	}

	matches := envVarPattern.FindStringSubmatch(strValue)
	if matches == nil {
		return value, nil
	}

	varName := matches[1]
	defaultPart := matches[2]

	envValue, exists := os.LookupEnv(varName)
	if exists {
		return envValue, nil
	}

	if defaultPart != "" {
		return strings.TrimPrefix(defaultPart, ":"), nil
	}

	return nil, fmt.Errorf("required environment variable not set: %s", varName)
}
