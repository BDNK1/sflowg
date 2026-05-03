package dsl

import (
	"context"
	"errors"
	"testing"

	"github.com/BDNK1/sflowg/core"
)

type fakeSubflowInvoker struct {
	flows map[string]runtime.Flow
	args  map[string]any
}

func (f *fakeSubflowInvoker) LookupFlow(name string) (*runtime.Flow, bool) {
	flow, ok := f.flows[name]
	if !ok {
		return nil, false
	}
	return &flow, true
}

func (f *fakeSubflowInvoker) InvokeSubflow(_ *runtime.Execution, _ *runtime.Flow, args map[string]any) (map[string]any, error) {
	f.args = args
	return map[string]any{"y": int64(2)}, nil
}

func newRunnerExecution() *runtime.Execution {
	return runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
}

func TestRunStep_BasicMapResult(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "charge",
		Body:   `{ok: true, amount: 10}`,
		Input:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	result, ok := output.Result.(map[string]any)
	if !ok || result["ok"] != true {
		t.Fatalf("expected map result with ok=true, got %#v", output.Result)
	}
}

func TestRunStep_ScalarResult(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "charge",
		Body:   `42`,
		Input:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if output.Result != int64(42) && output.Result != 42 {
		t.Fatalf("expected scalar result 42, got %#v", output.Result)
	}
}

func TestRunStep_EmptyBody(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "charge",
		Input:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if output.Result != nil || output.Response != nil || output.Next != "" {
		t.Fatalf("expected zero output, got %#v", output)
	}
}

func TestRunStep_ResponseSet(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "respond",
		Body:   `response.json({status: 201, body: {ok: true}})`,
		Input:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if output.Response == nil || output.Response.HandlerName != "http.json" {
		t.Fatalf("expected http.json response descriptor, got %#v", output.Response)
	}
	if output.Result != nil {
		t.Fatalf("expected nil result when response is set, got %#v", output.Result)
	}
}

func TestRunStep_NextCanonical(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID:     "route",
		Body:       `{__next: "finish", ok: true}`,
		Input:      map[string]any{},
		AllowsNext: true,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if output.Next != "finish" {
		t.Fatalf("expected next=finish, got %q", output.Next)
	}
	result, ok := output.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %#v", output.Result)
	}
	if _, exists := result["__next"]; exists {
		t.Fatalf("expected __next to be stripped from result, got %#v", result)
	}
}

func TestRunStep_IsolationGuarantee(t *testing.T) {
	exec := newRunnerExecution()
	exec.State().Store().Set("existing", "keep")
	runner := NewLocalStepRunner(NewStepExecutor())

	_, err := runner.RunStep(context.Background(), exec, runtime.StepInput{
		StepID: "charge",
		Body:   `{ok: true}`,
		Input:  exec.State().Store().Snapshot(),
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if _, ok := exec.State().Store().Get("charge"); ok {
		t.Fatal("expected workflow state to remain unchanged before merge")
	}
	if got, _ := exec.State().Store().Get("existing"); got != "keep" {
		t.Fatalf("expected existing value to remain, got %#v", got)
	}
}

func TestRunStep_ErrorPropagation(t *testing.T) {
	runner := NewLocalStepRunner(NewStepExecutor())
	_, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "charge",
		Body:   `raise("FAILED", "boom")`,
		Input:  map[string]any{},
	})
	var flowErr *runtime.FlowError
	if !errors.As(err, &flowErr) {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
}

func TestRunStep_ExtraEnvIsScopedToIsolatedStep(t *testing.T) {
	exec := newRunnerExecution()
	runner := NewLocalStepRunner(NewStepExecutor())
	output, err := runner.RunStep(context.Background(), exec, runtime.StepInput{
		StepID: "fallback",
		Body:   `{code: error.code}`,
		Input:  map[string]any{},
		ExtraEnv: map[string]any{
			"error": map[string]any{"code": "FAILED"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	result, ok := output.Result.(map[string]any)
	if !ok || result["code"] != "FAILED" {
		t.Fatalf("result = %#v", output.Result)
	}
	if _, ok := exec.State().Store().Get("error"); ok {
		t.Fatal("extra env leaked into parent execution store")
	}
}

func TestRunStep_FlowCallGlobal(t *testing.T) {
	invoker := &fakeSubflowInvoker{
		flows: map[string]runtime.Flow{
			"sub": {ID: "sub", Entrypoint: runtime.Entrypoint{Type: "flow"}},
		},
	}
	executor := NewStepExecutor()
	executor.SetSubflowInvoker(invoker)
	runner := NewLocalStepRunner(executor)

	output, err := runner.RunStep(context.Background(), newRunnerExecution(), runtime.StepInput{
		StepID: "call_sub",
		Body:   `flow.call("sub", {x: 1})`,
		Input:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	result, ok := output.Result.(map[string]any)
	if !ok || result["y"] != int64(2) {
		t.Fatalf("result = %#v", output.Result)
	}
	if invoker.args["x"] != int64(1) {
		t.Fatalf("args = %#v", invoker.args)
	}
}

func TestRunStep_CompiledModeFailsWhenBytecodeMissing(t *testing.T) {
	exec := newRunnerExecution()
	exec.Flow.DSLMode = runtime.DSLExecutionModeCompiled
	runner := NewLocalStepRunner(NewStepExecutor())

	_, err := runner.RunStep(context.Background(), exec, runtime.StepInput{
		StepID: "charge",
		Body:   `42`,
		Input:  map[string]any{},
	})
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}

	flowErr, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if flowErr.Code != string(runtime.ErrorCodeRuntimeError) {
		t.Fatalf("expected runtime error code, got %#v", flowErr)
	}
}

func TestRunStep_InterpretedModeIgnoresCompiledArtifacts(t *testing.T) {
	exec := newRunnerExecution()
	exec.Flow.DSLMode = runtime.DSLExecutionModeInterpreted
	runner := NewLocalStepRunner(NewStepExecutor())

	output, err := runner.RunStep(context.Background(), exec, runtime.StepInput{
		StepID:   "charge",
		Body:     `42`,
		Input:    map[string]any{},
		Compiled: struct{}{},
	})
	if err != nil {
		t.Fatalf("expected interpreted mode success, got %v", err)
	}
	if output.Result != int64(42) && output.Result != 42 {
		t.Fatalf("expected scalar result 42, got %#v", output.Result)
	}
}
