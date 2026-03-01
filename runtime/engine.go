package runtime

import "context"

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
	All() map[string]any
}

// StepExecutor executes a single flow step.
// The explicit ctx carries step-scoped timeout/cancellation for this invocation.
// The *Execution carries mutable flow state shared across all steps.
type StepExecutor interface {
	ExecuteStep(ctx context.Context, execution *Execution, step Step) (next string, err error)
}
