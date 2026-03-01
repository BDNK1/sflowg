package runtime

import (
	"context"
	"fmt"
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
	k, ok := key.(string)
	if !ok {
		return e.ctx.Value(key)
	}

	v, _ := e.Store.Get(k)
	return v
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

// Values returns the full context map for expression evaluation.
func (e *Execution) Values() map[string]any {
	return e.Store.All()
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

	// Merge properties: global properties first, then flow properties (flow overrides)
	for k, v := range globalProperties {
		exec.AddValue("properties."+k, resolveEnvVar(v))
	}

	for k, v := range flow.Properties {
		exec.AddValue("properties."+k, resolveEnvVar(v))
	}

	return exec
}

// envVarPattern matches ${VAR} and ${VAR:default} syntax
var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)(:[^}]*)?\}$`)

// resolveEnvVar resolves environment variables in property values
func resolveEnvVar(value any) any {
	strValue, ok := value.(string)
	if !ok {
		return value
	}

	matches := envVarPattern.FindStringSubmatch(strValue)
	if matches == nil {
		return value
	}

	varName := matches[1]
	defaultPart := matches[2]

	envValue, exists := os.LookupEnv(varName)
	if exists {
		return envValue
	}

	if defaultPart != "" {
		return strings.TrimPrefix(defaultPart, ":")
	}

	panic(fmt.Sprintf("Required environment variable not set: %s", varName))
}
