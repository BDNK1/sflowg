package runtime

type Flow struct {
	ID          string         `yaml:"id"`
	Entrypoint  Entrypoint     `yaml:"entrypoint"`
	Steps       []Step         `yaml:"steps"`
	Properties  map[string]any `yaml:"properties"`
	Return      Return         `yaml:"return"`
	OnErrorBody string         `yaml:"-"`
	Timeout     int            `yaml:"-"`
}

type Entrypoint struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config"`
}

type Step struct {
	ID             string         `yaml:"id"`
	Type           string         `yaml:"type"`
	Condition      string         `yaml:"condition,omitempty"`
	Args           map[string]any `yaml:"args"`
	Next           string         `yaml:"next,omitempty"`
	Retry          *RetryConfig   `yaml:"retry,omitempty"`
	Body           string         `yaml:"-"`
	Timeout        int            `yaml:"-"`
	FallbackBody   string         `yaml:"-"`
	CompensateBody string         `yaml:"-"`
}

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
