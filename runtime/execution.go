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

type Execution struct {
	ID                 string
	Store              ValueStore
	Flow               *Flow
	Container          *Container
	ResponseDescriptor *ResponseDescriptor
}

func (e *Execution) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (e *Execution) Done() <-chan struct{} {
	return nil
}

func (e *Execution) Err() error {
	if e.Container == nil {
		return nil
	}

	return nil
}

func (e *Execution) AddValue(k string, v any) {
	e.Store.Set(k, v)
}

func (e *Execution) Value(key any) any {
	k, ok := key.(string)
	if !ok {
		return nil
	}

	v, _ := e.Store.Get(k)
	return v
}

// Values returns the full context map for expression evaluation.
// This is a convenience accessor for backward compatibility.
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
	}

	// Merge properties: global properties first, then flow properties (flow overrides)
	// 1. Load global properties from flow-config.yaml
	for k, v := range globalProperties {
		exec.AddValue("properties."+k, resolveEnvVar(v))
	}

	// 2. Load flow properties (override globals)
	for k, v := range flow.Properties {
		exec.AddValue("properties."+k, resolveEnvVar(v))
	}

	return exec
}

// envVarPattern matches ${VAR} and ${VAR:default} syntax
var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)(:[^}]*)?\}$`)

// resolveEnvVar resolves environment variables in property values
// Supports:
//   - ${VAR}         - Required environment variable (panics if not set)
//   - ${VAR:default} - Optional environment variable with default
//   - literal        - Plain literal value (returned as-is)
func resolveEnvVar(value any) any {
	// Only process string values for env var substitution
	strValue, ok := value.(string)
	if !ok {
		return value // Return non-string values as-is
	}

	// Check if it matches env var pattern
	matches := envVarPattern.FindStringSubmatch(strValue)
	if matches == nil {
		// Not an env var pattern - return as-is
		return value
	}

	varName := matches[1]
	defaultPart := matches[2] // Will be ":default" or empty

	// Try to get environment variable
	envValue, exists := os.LookupEnv(varName)
	if exists {
		return envValue
	}

	// No env var found - check for default
	if defaultPart != "" {
		// Remove leading colon and return default value
		return strings.TrimPrefix(defaultPart, ":")
	}

	// Required env var not set - panic with clear message
	panic(fmt.Sprintf("Required environment variable not set: %s", varName))
}
