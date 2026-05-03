package dsl

import (
	"context"
	"testing"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/object"
)

func TestAsyncFutureObjectResolvesAttribute(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	exec.AsyncTasks().Register("prefetch", runtime.NewCompletedAsyncTask("prefetch", map[string]any{
		"body": map[string]any{"user": "alice"},
	}, nil))

	step := runtime.Step{ID: "consume", Body: `{user: prefetch.body.user}`}
	_, err := NewStepExecutor().ExecuteStep(context.Background(), exec.WithActiveStep("consume"), step)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}
	got, ok := exec.State().Store().Get("consume.user")
	if !ok || got != "alice" {
		t.Fatalf("consume.user = %v (present=%v), want alice", got, ok)
	}
}

func TestExecuteStepsAsyncConsumerAwaitsValue(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "prefetch", Async: true, Body: `{body: {user: "alice"}}`},
			{ID: "consume", Body: `{user: prefetch.body.user}`},
		},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, ok := exec.State().Store().Get("consume.user")
	if !ok || got != "alice" {
		t.Fatalf("consume.user = %v (present=%v), want alice", got, ok)
	}
}

func TestExecuteStepsAsyncSkippedRegistersNilFuture(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "prefetch", Async: true, Condition: "false", Body: `{body: {user: "alice"}}`},
			{ID: "consume", Body: `{missing: prefetch.body == nil}`},
		},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, ok := exec.State().Store().Get("consume.missing")
	if !ok || got != true {
		t.Fatalf("consume.missing = %v (present=%v), want true", got, ok)
	}
}

func TestAsyncFutureObjectNilAttributeIsNil(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	exec.AsyncTasks().Register("prefetch", runtime.NewCompletedAsyncTask("prefetch", nil, nil))

	step := runtime.Step{ID: "consume", Body: `{missing: prefetch.body == nil}`}
	_, err := NewStepExecutor().ExecuteStep(context.Background(), exec.WithActiveStep("consume"), step)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}
	got, ok := exec.State().Store().Get("consume.missing")
	if !ok || got != true {
		t.Fatalf("consume.missing = %v (present=%v), want true", got, ok)
	}
}

func TestAsyncFutureObjectPropagatesAwaitedError(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	exec.AsyncTasks().Register("prefetch", runtime.NewCompletedAsyncTask("prefetch", nil, &runtime.FlowError{
		Type:    runtime.ErrorTypePermanent,
		Code:    "PREFETCH_FAILED",
		Message: "boom",
		Step:    "prefetch",
	}))

	step := runtime.Step{ID: "consume", Body: `{user: prefetch.body.user}`}
	_, err := NewStepExecutor().ExecuteStep(context.Background(), exec.WithActiveStep("consume"), step)
	fe, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if fe.Code != "PREFETCH_FAILED" || fe.AwaitedFrom != "prefetch" || fe.Step != "consume" {
		t.Fatalf("unexpected awaited error: %#v", fe)
	}
}

func TestAsyncFutureObjectPropagatesAwaitedErrorInTruthinessContext(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	exec.AsyncTasks().Register("prefetch", runtime.NewCompletedAsyncTask("prefetch", nil, &runtime.FlowError{
		Type:    runtime.ErrorTypePermanent,
		Code:    "PREFETCH_FAILED",
		Message: "boom",
		Step:    "prefetch",
	}))

	step := runtime.Step{ID: "consume", Body: `{ok: !prefetch}`}
	_, err := NewStepExecutor().ExecuteStep(context.Background(), exec.WithActiveStep("consume"), step)
	fe, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if fe.Code != "PREFETCH_FAILED" || fe.AwaitedFrom != "prefetch" || fe.Step != "consume" {
		t.Fatalf("unexpected awaited error: %#v", fe)
	}
}

func TestAsyncFutureObjectDynamicAttrReturnsAwaitedErrorWithoutPanic(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	task := runtime.NewCompletedAsyncTask("prefetch", nil, &runtime.FlowError{
		Type:    runtime.ErrorTypePermanent,
		Code:    "PREFETCH_FAILED",
		Message: "boom",
		Step:    "prefetch",
	})
	future := newAsyncFutureObject(exec, "prefetch", task, "consume")
	attr, ok := future.GetAttr("body")
	if !ok {
		t.Fatal("expected dynamic attribute")
	}
	resolver, ok := attr.(object.AttrResolver)
	if !ok {
		t.Fatalf("attribute is %T, want object.AttrResolver", attr)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ResolveAttr panicked: %v", r)
		}
	}()
	_, err := resolver.ResolveAttr(context.Background(), "body")
	fe, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if fe.Code != "PREFETCH_FAILED" || fe.AwaitedFrom != "prefetch" || fe.Step != "consume" {
		t.Fatalf("unexpected awaited error: %#v", fe)
	}
}

func TestAsyncFutureObjectAwaitRespectsConsumerCancellation(t *testing.T) {
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
	task := runtime.NewAsyncTask("prefetch", func() {}, nil)
	future := newAsyncFutureObject(exec, "prefetch", task, "consume")
	attr, ok := future.GetAttr("body")
	if !ok {
		t.Fatal("expected dynamic attribute")
	}
	resolver, ok := attr.(object.AttrResolver)
	if !ok {
		t.Fatalf("attribute is %T, want object.AttrResolver", attr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolver.ResolveAttr(ctx, "body")
	fe, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if fe.Code != string(runtime.ErrorCodeContextCancelled) || fe.AwaitedFrom != "prefetch" || fe.Step != "consume" {
		t.Fatalf("unexpected awaited cancellation error: %#v", fe)
	}
}
