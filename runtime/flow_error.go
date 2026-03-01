package runtime

import "fmt"

// FlowErrorType classifies error severity and retry behavior.
type FlowErrorType string

const (
	// ErrorTypeTransient signals the operation can be retried.
	ErrorTypeTransient FlowErrorType = "transient"
	// ErrorTypePermanent signals the operation should not be retried.
	ErrorTypePermanent FlowErrorType = "permanent"
	// ErrorTypeTimeout signals the operation was cancelled by a deadline.
	ErrorTypeTimeout FlowErrorType = "timeout"
)

// FlowErrorCode identifies known runtime/framework error codes.
// DSL/user-defined codes may use any string value.
type FlowErrorCode string

const (
	// Framework-generated codes.
	ErrorCodeRuntimeError     FlowErrorCode = "RUNTIME_ERROR"
	ErrorCodeContextCancelled FlowErrorCode = "CONTEXT_CANCELLED"
	ErrorCodeDeadlineExceeded FlowErrorCode = "DEADLINE_EXCEEDED"

	// Default code used when DSL raise() is called without arguments.
	ErrorCodeRaise FlowErrorCode = "RAISE"
)

// FlowError is the canonical error type propagated through a flow execution.
// It is JSON-serializable so it can be used as a Temporal workflow payload.
type FlowError struct {
	Type    FlowErrorType  `json:"type"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Step    string         `json:"step"`
	Cause   any            `json:"cause,omitempty"`
	Retries int            `json:"retries"`
	Meta    map[string]any `json:"meta,omitempty"`
}

func (e *FlowError) Error() string {
	return fmt.Sprintf("[%s/%s] %s (step: %s, retries: %d)", e.Type, e.Code, e.Message, e.Step, e.Retries)
}

// ToMap converts the error to a map suitable for injection into Risor/expr-lang contexts.
func (e *FlowError) ToMap() map[string]any {
	return map[string]any{
		"type":    string(e.Type),
		"code":    e.Code,
		"message": e.Message,
		"step":    e.Step,
		"retries": e.Retries,
	}
}
