package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
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

// RunState holds the shared mutable state for a flow execution.
// It is created once per request and referenced by pointer in all derived Execution copies,
// so all scoped views (WithContext, WithActiveStep, etc.) read and write the same state.
type RunState struct {
	mu        sync.RWMutex
	store     ValueStore
	response  *ResponseDescriptor
	compStack []CompensationEntry
}

// Store returns the underlying ValueStore.
func (s *RunState) Store() ValueStore {
	return s.store
}

// SetResponse records the response produced by a step. Thread-safe.
func (s *RunState) SetResponse(r *ResponseDescriptor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.response = r
}

// Response returns the current response descriptor, or nil. Thread-safe.
func (s *RunState) Response() *ResponseDescriptor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.response
}

// AppendCompensation adds a compensation entry. Thread-safe.
func (s *RunState) AppendCompensation(entry CompensationEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compStack = append(s.compStack, entry)
}

// CompensationSnapshot returns a copy of the compensation stack in current order.
// The copy is safe to iterate while other code may still append. Thread-safe.
func (s *RunState) CompensationSnapshot() []CompensationEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CompensationEntry, len(s.compStack))
	copy(out, s.compStack)
	return out
}

// Execution is a lightweight, copyable scoped view of a running flow.
// It carries per-scope metadata (active step, plugin, path) and a context for
// deadline/cancellation. Shared mutable state lives in RunState, accessed via State().
//
// Derived copies are cheap — use WithContext / WithActiveStep / WithActivePath /
// WithActivePlugin to create a new scope without mutating the parent.
type Execution struct {
	ID           string
	Flow         *Flow
	Container    *Container
	ctx          context.Context // real context carrying deadline/cancellation
	activeStepID string
	activePath   SuccessPath
	activePlugin string
	state        *RunState // shared across all derived copies within one request
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
	if !ok || e.state == nil || e.state.store == nil {
		return e.ctx.Value(key)
	}

	if v, found := e.state.store.Get(k); found {
		return v
	}
	return e.ctx.Value(key)
}

// State returns the shared mutable state for this execution.
func (e *Execution) State() *RunState {
	return e.state
}

// WithContext returns a shallow copy of the Execution with a new embedded context.
// Use this to apply a per-step timeout without mutating the parent.
// Mirrors the http.Request.WithContext pattern.
func (e *Execution) WithContext(ctx context.Context) *Execution {
	copy := *e
	copy.ctx = ctx
	return &copy
}

// WithActiveStep returns a shallow copy with the given step ID in scope.
// Replaces the previous callback-based WithActiveStep(stepID, fn) pattern.
func (e *Execution) WithActiveStep(stepID string) *Execution {
	copy := *e
	copy.activeStepID = stepID
	return &copy
}

// WithActivePath returns a shallow copy with the given execution path in scope.
// Replaces the previous callback-based WithActivePath(path, fn) pattern.
func (e *Execution) WithActivePath(path SuccessPath) *Execution {
	copy := *e
	copy.activePath = path
	return &copy
}

// WithActivePlugin returns a shallow copy with the given plugin name in scope.
// Replaces the previous callback-based WithActivePlugin(pluginName, fn) pattern.
func (e *Execution) WithActivePlugin(pluginName string) *Execution {
	copy := *e
	copy.activePlugin = pluginName
	return &copy
}

func (e *Execution) AddValue(k string, v any) {
	e.state.store.Set(k, v)
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

func (e *Execution) Tracer() trace.Tracer {
	if e.Container == nil {
		return newNoopTracer()
	}
	return e.Container.Tracer()
}

func (e *Execution) Metrics() *Metrics {
	if e.Container == nil {
		return NewNoopMetrics()
	}
	return e.Container.Metrics()
}

// ActiveStepID returns the current step ID, if inside a step scope.
func (e *Execution) ActiveStepID() string {
	return e.activeStepID
}

// ActivePath returns the current execution path (primary/fallback), if set.
func (e *Execution) ActivePath() SuccessPath {
	return e.activePath
}

// Values returns the full context map for expression evaluation.
func (e *Execution) Values() map[string]any {
	return e.state.store.Snapshot()
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

// NewExecution creates a new execution for the given flow.
// Returns a pointer — all derived copies (WithContext, WithActiveStep, etc.) share
// the same RunState so state mutations are visible across all scopes.
func NewExecution(flow *Flow, container *Container, globalProperties map[string]any, store ValueStore) *Execution {
	id := uuid.New().String()
	exec := &Execution{
		ID:        id,
		Flow:      flow,
		Container: container,
		ctx:       context.Background(),
		state:     &RunState{store: store},
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
