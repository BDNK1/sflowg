package runtime

import (
	"context"
	"fmt"
)

// OnErrorExecutor is an optional interface that DSL step executors may implement
// to provide flow-level on_error and compensation handlers.
type OnErrorExecutor interface {
	ExecuteOnErrorHandler(execution *Execution, body string, fe *FlowError) error
	ExecuteCompensation(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error
}

// handleFailure runs compensation and on_error handling.
// Behavior:
//   - If no on_error is configured (or executable), returns original error.
//   - If on_error sets a response and does not raise, swallows the error.
//   - If on_error succeeds without a response, returns the original error.
//   - If on_error raises/returns an error, returns handler error instead of original.
func (e *Executor) handleFailure(execution *Execution, fe *FlowError) error {
	e.runCompensations(execution)
	handled, handlerErr := e.runOnErrorHandler(execution, fe)
	if !handled {
		return fe
	}
	if handlerErr != nil {
		return handlerErr
	}
	return nil
}

// HandleBoundaryError runs the flow-level on_error handler for an error raised
// before any step has executed, such as input contract validation.
func (e *Executor) HandleBoundaryError(execution *Execution, fe *FlowError) (bool, error) {
	handled, handlerErr := e.runOnErrorHandler(execution, fe)
	if handlerErr != nil {
		return handled, handlerErr
	}
	return handled, nil
}

// runCompensations iterates the CompensationStack in LIFO order and executes
// each compensation body. Failures are logged but do not stop remaining compensations.
// Uses a detached context so compensation DB/HTTP calls complete even if the flow
// context was already cancelled (e.g. by a timeout).
func (e *Executor) runCompensations(execution *Execution) {
	oee, ok := e.stepExecutor.(OnErrorExecutor)
	if !ok {
		return
	}

	safeExec := execution.WithContext(context.WithoutCancel(execution))
	log := safeExec.Logger()
	stack := execution.State().CompensationSnapshot()
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		log.Info(fmt.Sprintf("Running compensation for step %s (path: %s)", entry.StepID, entry.Path))
		if err := e.runIsolatedCompensation(safeExec, oee, entry); err != nil {
			log.Error(fmt.Sprintf("Compensation failed for step %s", entry.StepID), "error", err)
			continue
		}
	}
}

// runOnErrorHandler executes the flow-level on_error body if one is defined.
// Uses a detached context so the handler can complete (set response, update DB, etc.)
// even when the original flow context has already been cancelled by a timeout.
func (e *Executor) runOnErrorHandler(execution *Execution, fe *FlowError) (handled bool, handlerErr *FlowError) {
	if execution.Flow.OnErrorBody == "" {
		return false, nil
	}
	if _, ok := e.stepExecutor.(OnErrorExecutor); !ok {
		return false, nil
	}

	safeCtx := context.WithoutCancel(execution)
	log := execution.Logger()
	log.Info("Running flow-level on_error handler", "error_code", fe.Code)
	safeExec := execution.WithContext(safeCtx)
	output, err, handled := e.runIsolatedOnError(safeExec, fe)
	if !handled {
		return false, nil
	}
	if err != nil {
		log.Error("on_error handler itself failed", "error", err)
		return true, err
	}
	if responseErr := validateRecoveryResponse(execution, output.Response); responseErr != nil {
		return true, responseErr
	}
	applyOnErrorOutput(execution, output)
	return true, nil
}

func (e *Executor) runIsolatedOnError(execution *Execution, fe *FlowError) (RecoveryOutput, *FlowError, bool) {
	oee := e.stepExecutor.(OnErrorExecutor)

	isolatedExec := execution.WithIsolatedState(cloneRunStateSnapshot(execution.State().Store().Snapshot()))
	err := oee.ExecuteOnErrorHandler(isolatedExec, execution.Flow.OnErrorBody, fe)
	if err != nil {
		return RecoveryOutput{}, toFlowError(err, "on_error", 0), true
	}

	return collectRecoveryOutput(isolatedExec.State()), nil, true
}

func (e *Executor) runIsolatedCompensation(execution *Execution, oee OnErrorExecutor, entry CompensationEntry) error {
	isolatedExec := execution.WithIsolatedState(cloneRunStateSnapshot(execution.State().Store().Snapshot()))
	return oee.ExecuteCompensation(isolatedExec, entry.Body, entry.StepID, entry.Path, entry.Compiled)
}

func applyOnErrorOutput(execution *Execution, output RecoveryOutput) {
	if output.Response != nil {
		execution.State().SetResponse(output.Response)
	}
	mergeStoreSnapshot(execution.State().Store(), output.Store)
}

func collectRecoveryOutput(state *RunState) RecoveryOutput {
	return RecoveryOutput{
		Response: state.Response(),
		Store:    state.Store().Snapshot(),
	}
}
