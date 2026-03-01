package dsl

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/BDNK1/sflowg/runtime"
)

// StepExecutor executes DSL step bodies via the Risor interpreter.
// The *Execution is passed as the context.Context to Risor so that
// per-step and flow-level timeouts propagate into the interpreter.
type StepExecutor struct {
	interpreter *Interpreter
	l           *slog.Logger
}

func NewStepExecutor(l *slog.Logger) *StepExecutor {
	return &StepExecutor{
		interpreter: &Interpreter{},
		l:           l,
	}
}

func (e *StepExecutor) ExecuteStep(ctx context.Context, execution *runtime.Execution, step runtime.Step) (string, error) {
	if step.Body == "" {
		return "", nil
	}

	globals := e.buildEnv(execution)

	e.l.InfoContext(execution, fmt.Sprintf("Executing DSL step: %s", step.ID))

	// Pass execution as context.Context so Risor honours deadline/cancellation.
	result, err := e.interpreter.Eval(ctx, step.Body, globals)
	if err != nil {
		// Preserve FlowError values raised by DSL code.
		if _, ok := err.(*runtime.FlowError); !ok {
			// Context cancellation/deadline from interpreter execution should
			// remain timeout-classified for retry/on_error policies.
			if errors.Is(err, context.DeadlineExceeded) {
				return "", &runtime.FlowError{
					Type:    runtime.ErrorTypeTimeout,
					Code:    string(runtime.ErrorCodeDeadlineExceeded),
					Message: err.Error(),
					Step:    step.ID,
					Cause:   err,
				}
			}
			if errors.Is(err, context.Canceled) {
				return "", &runtime.FlowError{
					Type:    runtime.ErrorTypeTimeout,
					Code:    string(runtime.ErrorCodeContextCancelled),
					Message: err.Error(),
					Step:    step.ID,
					Cause:   err,
				}
			}

			// Wrap other interpreter failures as permanent runtime errors.
			return "", &runtime.FlowError{
				Type:    runtime.ErrorTypePermanent,
				Code:    string(runtime.ErrorCodeRuntimeError),
				Message: err.Error(),
				Step:    step.ID,
				Cause:   err,
			}
		}
		return "", err
	}

	// Store the step result under the step ID, but skip if a response
	// descriptor was set (the step produced a response, not a stored value).
	if result != nil && execution.ResponseDescriptor == nil {
		if m, ok := result.(map[string]any); ok {
			execution.Store.SetNested(step.ID, m)
		} else {
			execution.Store.Set(step.ID, result)
		}
	}

	return "", nil
}

// ExecuteOnErrorHandler runs the flow-level on_error Risor body.
// The current FlowError is injected as `error` so the handler can inspect it.
func (e *StepExecutor) ExecuteOnErrorHandler(execution *runtime.Execution, body string, fe *runtime.FlowError) error {
	globals := e.buildEnv(execution)
	globals["error"] = fe.ToMap()
	_, err := e.interpreter.Eval(execution, body, globals)
	return err
}

// ExecuteCompensation runs a compensation Risor body for a previously-succeeded step.
// Injects `compensation.step` and `compensation.path` so the body can apply the
// correct undo logic depending on which execution branch produced side-effects.
func (e *StepExecutor) ExecuteCompensation(execution *runtime.Execution, body string, stepID string, path runtime.SuccessPath) error {
	globals := e.buildEnv(execution)
	globals["compensation"] = map[string]any{
		"step": stepID,
		"path": string(path),
	}
	_, err := e.interpreter.Eval(execution, body, globals)
	return err
}

// buildEnv assembles the env map for a step's Risor evaluation.
func (e *StepExecutor) buildEnv(execution *runtime.Execution) map[string]any {
	globals := make(map[string]any)

	for k, v := range execution.Store.All() {
		globals[k] = v
	}

	pluginGlobals := BuildPluginGlobals(execution)
	for k, v := range pluginGlobals {
		globals[k] = v
	}

	responseGlobals := BuildResponseGlobals(execution)
	for k, v := range responseGlobals {
		globals[k] = v
	}

	globals["sprintf"] = fmt.Sprintf
	globals["base64_encode"] = func(v any) string {
		return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", v)))
	}

	// raise() lets DSL code signal a FlowError explicitly.
	// Signature: raise(type, code, message)  or  raise(code, message)
	globals["raise"] = func(args ...any) (any, error) {
		return nil, parseRaiseArgs(args...)
	}

	return globals
}

// parseRaiseArgs converts variadic raise() arguments into a *FlowError.
//
//	raise("transient", "PAYMENT_TIMEOUT", "upstream timed out")
//	raise("PAYMENT_TIMEOUT", "upstream timed out")   // defaults to permanent
func parseRaiseArgs(args ...any) *runtime.FlowError {
	fe := &runtime.FlowError{
		Type: runtime.ErrorTypePermanent,
	}
	switch len(args) {
	case 0:
		fe.Code = string(runtime.ErrorCodeRaise)
		fe.Message = "raise() called with no arguments"
	case 1:
		if parsed, ok := asFlowError(args[0]); ok {
			return parsed
		}
		fe.Code = fmt.Sprintf("%v", args[0])
		fe.Message = fe.Code
	case 2:
		fe.Code = fmt.Sprintf("%v", args[0])
		fe.Message = fmt.Sprintf("%v", args[1])
	default:
		fe.Type = runtime.FlowErrorType(fmt.Sprintf("%v", args[0]))
		fe.Code = fmt.Sprintf("%v", args[1])
		fe.Message = fmt.Sprintf("%v", args[2])
	}
	return fe
}

func asFlowError(v any) (*runtime.FlowError, bool) {
	if existing, ok := v.(*runtime.FlowError); ok {
		return existing, true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	fe := &runtime.FlowError{
		Type: runtime.ErrorTypePermanent,
	}
	if t, ok := m["type"].(string); ok && t != "" {
		fe.Type = runtime.FlowErrorType(t)
	}
	if c, ok := m["code"].(string); ok {
		fe.Code = c
	}
	if msg, ok := m["message"].(string); ok {
		fe.Message = msg
	}
	if s, ok := m["step"].(string); ok {
		fe.Step = s
	}
	switch r := m["retries"].(type) {
	case int:
		fe.Retries = r
	case int64:
		fe.Retries = int(r)
	case float64:
		fe.Retries = int(r)
	}
	if fe.Code == "" {
		fe.Code = string(runtime.ErrorCodeRaise)
	}
	if fe.Message == "" {
		fe.Message = fe.Code
	}
	return fe, true
}
