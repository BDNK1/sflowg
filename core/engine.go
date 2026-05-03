package runtime

import (
	"context"
	"fmt"
	"strings"
)

type DSLExecutionMode string

const (
	DSLExecutionModeInterpreted DSLExecutionMode = "interpreted"
	DSLExecutionModeCompiled    DSLExecutionMode = "compiled"
)

// FlowLoader loads flow definitions from files.
type FlowLoader interface {
	Extensions() []string
	Load(filePath string) (Flow, error)
}

// FlowCompiler pre-compiles flows after load time using runtime container context.
type FlowCompiler interface {
	CompileFlow(ctx context.Context, flow *Flow, container *Container) error
}

// ExpressionEvaluator evaluates expressions within a given execution.
// The *Execution carries both the variable namespace (via Values()) and the
// deadline/cancellation signal (it implements context.Context), so a single
// parameter covers both concerns.
type ExpressionEvaluator interface {
	Eval(execution *Execution, expression string) (any, error)
	EvalWithEnv(execution *Execution, expression string, extraVars map[string]any) (any, error)
}

// ValueStore manages execution state storage and retrieval.
type ValueStore interface {
	Set(key string, value any)
	Get(key string) (any, bool)
	SetNested(prefix string, value any)
	Snapshot() map[string]any // safe copy for external consumers
}

// StepExecutor executes a single flow step.
// The explicit ctx carries step-scoped timeout/cancellation for this invocation.
// The *Execution carries mutable flow state shared across all steps.
type StepExecutor interface {
	ExecuteStep(ctx context.Context, execution *Execution, step Step) (next string, err error)
}

// StepInput is the canonical input passed to a single isolated step execution.
type StepInput struct {
	StepID     string         `json:"step_id"`
	Body       string         `json:"body"`
	Input      map[string]any `json:"input"`
	ExtraEnv   map[string]any `json:"extra_env,omitempty"`
	Timeout    int            `json:"timeout,omitempty"`
	Path       SuccessPath    `json:"path,omitempty"`
	Compiled   any            `json:"-"`
	AllowsNext bool           `json:"-"`
}

// StepOutput is the canonical output produced by a single isolated step execution.
type StepOutput struct {
	Result   any                 `json:"result,omitempty"`
	Response *ResponseDescriptor `json:"response,omitempty"`
	Next     string              `json:"next,omitempty"`
}

// RecoveryOutput is the canonical output collected from isolated on_error execution.
type RecoveryOutput struct {
	Response *ResponseDescriptor `json:"response,omitempty"`
	Store    map[string]any      `json:"store,omitempty"`
}

// StepRunner runs one step against isolated state and returns its canonical output.
type StepRunner interface {
	RunStep(ctx context.Context, execution *Execution, input StepInput) (StepOutput, error)
}

// BuildStepInput constructs the current canonical input shape for a step.
func BuildStepInput(execution *Execution, step Step, path SuccessPath) (StepInput, error) {
	return BuildStepInputWithExtra(execution, step, path, nil)
}

// BuildStepInputWithExtra constructs StepInput and overlays scoped values that
// should be visible only to this isolated step body.
func BuildStepInputWithExtra(execution *Execution, step Step, path SuccessPath, extra map[string]any) (StepInput, error) {
	input := execution.State().Store().Snapshot()
	if execution.DSLMode() == DSLExecutionModeCompiled {
		if strings.TrimSpace(step.Body) != "" && step.StoreKeys == nil {
			return StepInput{}, fmt.Errorf("compiled mode invariant violated: step %q is missing compiled store keys", step.ID)
		}
		if step.StoreKeys != nil {
			input = boundedSnapshot(execution.State().Store(), step.StoreKeys)
		}
	}

	return StepInput{
		StepID:     step.ID,
		Body:       step.Body,
		Input:      input,
		ExtraEnv:   extra,
		Timeout:    step.Timeout,
		Path:       path,
		Compiled:   step.Compiled,
		AllowsNext: step.AllowsNext,
	}, nil
}

func boundedSnapshot(store ValueStore, keys []string) map[string]any {
	full := store.Snapshot()
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		result[key] = full[key]
	}
	return result
}
