package dsl

import (
	"context"
	"errors"
	"testing"

	"github.com/BDNK1/sflowg/runtime"
)

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
	if output.Result != nil || output.Response != nil || output.Next != "" || len(output.SideEffects) != 0 {
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
		StepID: "route",
		Body:   `{__next: "finish", ok: true}`,
		Input:  map[string]any{},
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
