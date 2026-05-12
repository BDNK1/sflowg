package runtime

import (
	"context"
	"errors"
)

// flowError wraps a context error as a FlowError for unified error handling.
func (e *Executor) flowError(_ *Execution, stepID string, err error) *FlowError {
	if errors.Is(err, context.DeadlineExceeded) {
		return &FlowError{Type: ErrorTypeTimeout, Code: string(ErrorCodeDeadlineExceeded), Message: err.Error(), Step: stepID}
	}
	return &FlowError{Type: ErrorTypeTimeout, Code: string(ErrorCodeContextCancelled), Message: err.Error(), Step: stepID}
}

// toFlowError converts any error to a *FlowError, preserving existing FlowErrors.
func toFlowError(err error, stepID string, attempt int) *FlowError {
	if errors.Is(err, context.DeadlineExceeded) {
		return &FlowError{
			Type:    ErrorTypeTimeout,
			Code:    string(ErrorCodeDeadlineExceeded),
			Message: err.Error(),
			Step:    stepID,
			Retries: attempt,
		}
	}
	if errors.Is(err, context.Canceled) {
		return &FlowError{
			Type:    ErrorTypeTimeout,
			Code:    string(ErrorCodeContextCancelled),
			Message: err.Error(),
			Step:    stepID,
			Retries: attempt,
		}
	}

	var fe *FlowError
	if errors.As(err, &fe) {
		if fe.Step == "" {
			fe.Step = stepID
		}
		return fe
	}
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: err.Error(),
		Step:    stepID,
		Retries: attempt,
	}
}
