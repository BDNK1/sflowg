package runtime

import (
	"context"
	"time"
)

// FlowLoader loads flow definitions from files.
type FlowLoader interface {
	Extensions() []string
	Load(filePath string) (Flow, error)
}

// ExpressionEvaluator evaluates expressions within a given execution.
// The *Execution carries both the variable namespace (via Values()) and the
// deadline/cancellation signal (it implements context.Context), so a single
// parameter covers both concerns.
type ExpressionEvaluator interface {
	Eval(execution *Execution, expression string) (any, error)
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

// SideEffect records one external plugin interaction that occurred during execution.
type SideEffect struct {
	Plugin    string        `json:"plugin"`
	Method    string        `json:"method"`
	Input     any           `json:"input,omitempty"`
	Output    any           `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// StepInput is the canonical input passed to a single isolated step execution.
type StepInput struct {
	StepID  string         `json:"step_id"`
	Body    string         `json:"body"`
	Input   map[string]any `json:"input"`
	Timeout int            `json:"timeout,omitempty"`
	Path    SuccessPath    `json:"path,omitempty"`
}

// StepOutput is the canonical output produced by a single isolated step execution.
type StepOutput struct {
	Result      any                 `json:"result,omitempty"`
	Response    *ResponseDescriptor `json:"response,omitempty"`
	Next        string              `json:"next,omitempty"`
	SideEffects []SideEffect        `json:"side_effects,omitempty"`
}

// RecoveryOutput is the canonical output collected from isolated on_error and
// compensation execution.
type RecoveryOutput struct {
	Response    *ResponseDescriptor `json:"response,omitempty"`
	SideEffects []SideEffect        `json:"side_effects,omitempty"`
	Store       map[string]any      `json:"store,omitempty"`
}

// StepRunner runs one step against isolated state and returns its canonical output.
type StepRunner interface {
	RunStep(ctx context.Context, execution *Execution, input StepInput) (StepOutput, error)
}

// BuildStepInput constructs the current canonical input shape for a step.
// Phase 2 will swap the full snapshot with bounded keys without changing executor logic.
func BuildStepInput(execution *Execution, step Step, path SuccessPath) StepInput {
	return StepInput{
		StepID:  step.ID,
		Body:    step.Body,
		Input:   execution.State().Store().Snapshot(),
		Timeout: step.Timeout,
		Path:    path,
	}
}
