package runtime

type Flow struct {
	ID               string           `yaml:"id"`
	Entrypoint       Entrypoint       `yaml:"entrypoint"`
	Nodes            []FlowNode       `yaml:"-" json:"-"`
	Steps            []Step           `yaml:"steps"`
	Properties       map[string]any   `yaml:"properties"`
	Return           Return           `yaml:"return"`
	DSLMode          DSLExecutionMode `yaml:"-" json:"-"`
	ResponseContract ResponseContract `yaml:"-" json:"-"`
	ResponseSubtypes []string         `yaml:"-" json:"-"`
	OnErrorBody      string           `yaml:"-"`
	OnErrorCompiled  any              `yaml:"-" json:"-"`
	Timeout          int              `yaml:"-"`
}

type Entrypoint struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
	Input  *InputContract `yaml:"-" json:"-"`
}

type Step struct {
	ID                 string         `yaml:"id"`
	Type               string         `yaml:"type"`
	Async              bool           `yaml:"-" json:"-"`
	Condition          string         `yaml:"condition,omitempty"`
	Args               map[string]any `yaml:"args"`
	Next               string         `yaml:"next,omitempty"`
	Retry              *RetryConfig   `yaml:"retry,omitempty"`
	Body               string         `yaml:"-"`
	Timeout            int            `yaml:"-"`
	StoreKeys          []string       `yaml:"-" json:"-"`
	AsyncDeps          []string       `yaml:"-" json:"-"`
	Compiled           any            `yaml:"-" json:"-"`
	FallbackBody       string         `yaml:"-"`
	FallbackStoreKeys  []string       `yaml:"-" json:"-"`
	FallbackCompiled   any            `yaml:"-" json:"-"`
	CompensateBody     string         `yaml:"-"`
	CompensateCompiled any            `yaml:"-" json:"-"`
	AllowsNext         bool           `yaml:"-" json:"-"`
}

type FlowNodeKind string

const (
	FlowNodeStep     FlowNodeKind = "step"
	FlowNodeParallel FlowNodeKind = "parallel"

	InternalParallelNodePrefix = "__parallel_"
)

type FlowNode struct {
	ID       string         `yaml:"-" json:"-"`
	Kind     FlowNodeKind   `yaml:"-" json:"-"`
	Step     *Step          `yaml:"-" json:"-"`
	Parallel *ParallelBlock `yaml:"-" json:"-"`
}

type ParallelBlock struct {
	Options  ParallelOptions `yaml:"-" json:"-"`
	Branches []Step          `yaml:"-" json:"-"`
}

type ParallelOptions struct {
	MaxInFlight int           `yaml:"-" json:"-"`
	OnFailure   OnFailureMode `yaml:"-" json:"-"`
}

type OnFailureMode string

const (
	OnFailureWaitAll  OnFailureMode = "wait_all"
	OnFailureFailFast OnFailureMode = "fail_fast"
)

type Return struct {
	Type string         `yaml:"type"`
	Args map[string]any `yaml:"args"`
	Body string         `yaml:"-"` // DSL: raw Risor code for return (ignored by YAML)
}

// RetryConfig controls retry behavior for a step.
// Backward compatibility: maxRetries maps to MaxAttempts, condition maps to When,
// backoff: true maps to Backoff: "exponential".
type RetryConfig struct {
	MaxAttempts  int      `yaml:"maxAttempts"`
	Delay        int      `yaml:"delay"`    // base delay in ms
	Backoff      string   `yaml:"backoff"`  // "none" | "linear" | "exponential"
	MaxDelay     int      `yaml:"maxDelay"` // ms; 0 = no cap
	Jitter       bool     `yaml:"jitter"`
	When         string   `yaml:"when"`         // Risor expression evaluated with `error` in scope
	NonRetryable []string `yaml:"nonRetryable"` // error codes that must not be retried
}
