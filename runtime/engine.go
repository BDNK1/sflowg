package runtime

// FlowLoader loads flow definitions from files.
// YAML implementation reads *.yaml files, DSL implementation will read *.flow files.
type FlowLoader interface {
	// Extensions returns the file glob patterns this loader handles (e.g., "*.yaml", "*.flow")
	Extensions() []string
	// Load parses a file into a Flow definition
	Load(filePath string) (Flow, error)
}

// ExpressionEvaluator evaluates expressions within a given context.
// YAML implementation uses expr-lang with flat key formatting.
// DSL implementation will use Risor with native dot access.
type ExpressionEvaluator interface {
	Eval(expression string, context map[string]any) (any, error)
}

// ValueStore manages execution state storage and retrieval.
// YAML implementation flattens keys (step.result.field → step_result_field).
// DSL implementation will use nested maps with native dot access.
type ValueStore interface {
	// Set stores a value by key
	Set(key string, value any)
	// Get retrieves a value by key
	Get(key string) (any, bool)
	// SetNested stores a value and recursively expands nested maps/arrays
	SetNested(prefix string, value any)
	// All returns the full context map for expression evaluation
	All() map[string]any
}

// StepExecutor executes a single flow step.
// YAML implementation dispatches by step type (assign/switch/return/task).
// DSL implementation executes Risor code from the step body.
type StepExecutor interface {
	// ExecuteStep runs a step and returns the next step ID (for branching) or empty string.
	ExecuteStep(execution *Execution, step Step) (next string, err error)
}
