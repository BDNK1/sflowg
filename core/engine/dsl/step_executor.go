package dsl

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/bytecode"
)

// StepExecutor executes DSL step bodies via the Risor interpreter.
// The *Execution is passed as the context.Context to Risor so that
// per-step and flow-level timeouts propagate into the interpreter.
type StepExecutor struct {
	interpreter *Interpreter
	subflows    runtime.SubflowInvoker
}

func NewStepExecutor() *StepExecutor {
	return &StepExecutor{
		interpreter: &Interpreter{},
	}
}

func (e *StepExecutor) SetSubflowInvoker(invoker runtime.SubflowInvoker) {
	e.subflows = invoker
}

func (e *StepExecutor) ExecuteStep(ctx context.Context, execution *runtime.Execution, step runtime.Step) (string, error) {
	if step.Body == "" {
		return "", nil
	}

	globals := e.buildEnv(execution)

	execution.Logger().Info(fmt.Sprintf("Executing DSL step: %s", step.ID))

	result, err := e.evalStep(ctx, execution.DSLMode(), step.ID, step.Body, step.Compiled, globals)
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
	var next string
	if result != nil && execution.State().Response() == nil {
		if m, ok := result.(map[string]any); ok {
			if n, exists := m["__next"]; exists {
				next = fmt.Sprintf("%v", n)
				delete(m, "__next")
			}
			if len(m) > 0 {
				execution.State().Store().SetNested(step.ID, m)
			}
		} else {
			execution.State().Store().Set(step.ID, result)
		}
	}

	return next, nil
}

// ExecuteOnErrorHandler runs the flow-level on_error Risor body.
// The current FlowError is injected as `error` so the handler can inspect it.
func (e *StepExecutor) ExecuteOnErrorHandler(execution *runtime.Execution, body string, fe *runtime.FlowError) error {
	globals := e.buildEnv(execution)
	globals["error"] = fe.ToMap()
	_, err := e.evalStep(execution, execution.DSLMode(), "on_error", body, execution.Flow.OnErrorCompiled, globals)
	return err
}

// ExecuteCompensation runs a compensation Risor body for a previously-succeeded step.
// Injects `compensation.step` and `compensation.path` so the body can apply the
// correct undo logic depending on which execution branch produced side-effects.
func (e *StepExecutor) ExecuteCompensation(execution *runtime.Execution, body string, stepID string, path runtime.SuccessPath, compiled any) error {
	globals := e.buildEnv(execution)
	globals["compensation"] = map[string]any{
		"step": stepID,
		"path": string(path),
	}
	_, err := e.evalStep(execution, execution.DSLMode(), stepID, body, compiled, globals)
	return err
}

func (e *StepExecutor) evalStep(ctx context.Context, mode runtime.DSLExecutionMode, stepID string, body string, compiled any, globals map[string]any) (any, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	if mode == runtime.DSLExecutionModeCompiled {
		code, ok := compiled.(*bytecode.Code)
		if !ok {
			return nil, &runtime.FlowError{
				Type:    runtime.ErrorTypePermanent,
				Code:    string(runtime.ErrorCodeRuntimeError),
				Message: fmt.Sprintf("compiled mode invariant violated: step %q is missing compiled bytecode", stepID),
				Step:    stepID,
			}
		}
		preSeedMissingKeys(globals, code)
		return e.interpreter.Run(ctx, code, globals)
	}
	return e.interpreter.Eval(ctx, body, globals)
}

func preSeedMissingKeys(globals map[string]any, code *bytecode.Code) {
	for _, key := range code.EnvKeys() {
		if _, exists := globals[key]; !exists {
			globals[key] = nil
		}
	}
}

// buildEnv assembles the env map for a step's Risor evaluation.
func (e *StepExecutor) buildEnv(execution *runtime.Execution) map[string]any {
	globals := make(map[string]any)

	for k, v := range execution.State().Store().Snapshot() {
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

	flowGlobals := BuildFlowCallGlobals(e.subflows, execution)
	for k, v := range flowGlobals {
		globals[k] = v
	}

	logGlobals := BuildLogGlobals(execution)
	for k, v := range logGlobals {
		globals[k] = v
	}

	metricGlobals := BuildMetricGlobals(execution)
	for k, v := range metricGlobals {
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
