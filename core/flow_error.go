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
	ErrorCodeSchemaViolation  FlowErrorCode = "SCHEMA_VIOLATION"
	ErrorCodeParallelFailure  FlowErrorCode = "PARALLEL_FAILURE"

	// Default code used when DSL raise() is called without arguments.
	ErrorCodeRaise FlowErrorCode = "RAISE"
)

// FlowError is the canonical error type propagated through a flow execution.
// It is JSON-serializable so it can be used as a Temporal workflow payload.
type FlowError struct {
	Type        FlowErrorType  `json:"type"`
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Step        string         `json:"step"`
	Cause       any            `json:"cause,omitempty"`
	Retries     int            `json:"retries"`
	Meta        map[string]any `json:"meta,omitempty"`
	AwaitedFrom string         `json:"awaited_from,omitempty"`
	Failures    []FlowFailure  `json:"failures,omitempty"`
}

type FlowFailure struct {
	Branch string          `json:"branch,omitempty"`
	Step   string          `json:"step,omitempty"`
	Error  FlowErrorDetail `json:"error"`
}

type FlowErrorDetail struct {
	Type        FlowErrorType  `json:"type"`
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Step        string         `json:"step"`
	Retries     int            `json:"retries"`
	Meta        map[string]any `json:"meta,omitempty"`
	AwaitedFrom string         `json:"awaited_from,omitempty"`
}

func (e *FlowError) Error() string {
	return fmt.Sprintf("[%s/%s] %s (step: %s, retries: %d)", e.Type, e.Code, e.Message, e.Step, e.Retries)
}

// ToMap converts the error to a map suitable for injection into Risor/expr-lang contexts.
func (e *FlowError) ToMap() map[string]any {
	out := map[string]any{
		"type":    string(e.Type),
		"code":    e.Code,
		"message": e.Message,
		"step":    e.Step,
		"retries": e.Retries,
	}
	if len(e.Meta) > 0 {
		out["meta"] = e.Meta
	}
	if e.AwaitedFrom != "" {
		out["awaited_from"] = e.AwaitedFrom
	}
	if len(e.Failures) > 0 {
		failures := make([]map[string]any, 0, len(e.Failures))
		for _, failure := range e.Failures {
			item := map[string]any{
				"error": flowErrorDetailToMap(failure.Error),
			}
			if failure.Branch != "" {
				item["branch"] = failure.Branch
			}
			if failure.Step != "" {
				item["step"] = failure.Step
			}
			failures = append(failures, item)
		}
		out["failures"] = failures
	}
	return out
}

func FlowErrorDetailFrom(fe *FlowError) FlowErrorDetail {
	if fe == nil {
		return FlowErrorDetail{}
	}
	return FlowErrorDetail{
		Type:        fe.Type,
		Code:        fe.Code,
		Message:     fe.Message,
		Step:        fe.Step,
		Retries:     fe.Retries,
		Meta:        fe.Meta,
		AwaitedFrom: fe.AwaitedFrom,
	}
}

func flowErrorDetailToMap(detail FlowErrorDetail) map[string]any {
	out := map[string]any{
		"type":    string(detail.Type),
		"code":    detail.Code,
		"message": detail.Message,
		"step":    detail.Step,
		"retries": detail.Retries,
	}
	if len(detail.Meta) > 0 {
		out["meta"] = detail.Meta
	}
	if detail.AwaitedFrom != "" {
		out["awaited_from"] = detail.AwaitedFrom
	}
	return out
}

func NewParallelFailure(stepID string, failures []FlowFailure) *FlowError {
	if len(failures) == 0 {
		return nil
	}
	return &FlowError{
		Type:     ErrorTypePermanent,
		Code:     string(ErrorCodeParallelFailure),
		Message:  "multiple parallel branches failed",
		Step:     stepID,
		Failures: failures,
	}
}

func cloneFlowErrorForAwait(origin *FlowError, asyncStepID string, consumerStepID string) *FlowError {
	if origin == nil {
		return nil
	}
	clone := *origin
	clone.AwaitedFrom = asyncStepID
	clone.Step = consumerStepID
	return &clone
}
