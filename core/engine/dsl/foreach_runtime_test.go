package dsl

import (
	"testing"

	"github.com/BDNK1/sflowg/core"
)

func TestExecuteStepsForeachAsyncConsumerAwaitsLocalValue(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "foreach_async",
		Nodes: []runtime.FlowNode{{
			ID:   "__foreach_1",
			Kind: runtime.FlowNodeForeach,
			Foreach: &runtime.ForeachBlock{
				Expr:    "items",
				ItemVar: "item",
				Steps: []runtime.Step{
					{ID: "prefetch", Async: true, Body: `{value: item.value + "-async"}`},
					{ID: "consume", Body: `{value: prefetch.value}`},
				},
				Collects: []runtime.ForeachCollect{{Expr: "consume.value", Alias: "values"}},
			},
		}},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())
	exec.State().Store().SetNested("items", []any{
		map[string]any{"value": "a"},
		map[string]any{"value": "b"},
	})

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	want := []any{"a-async", "b-async"}
	if len(got.([]any)) != len(want) || got.([]any)[0] != want[0] || got.([]any)[1] != want[1] {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
	if _, ok := exec.AsyncTasks().Get("prefetch"); ok {
		t.Fatal("local foreach async task leaked to parent async scope")
	}
}

func TestExecuteStepsForeachCanReadPriorParentAsyncTask(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "foreach_parent_async",
		Nodes: []runtime.FlowNode{
			{ID: "parent_async", Kind: runtime.FlowNodeStep, Step: &runtime.Step{ID: "parent_async", Async: true, Body: `{prefix: "parent"}`}},
			{
				ID:   "__foreach_1",
				Kind: runtime.FlowNodeForeach,
				Foreach: &runtime.ForeachBlock{
					Expr:     "items",
					ItemVar:  "item",
					Collects: []runtime.ForeachCollect{{Expr: "parent_async.prefix + item", Alias: "values"}},
				},
			},
		},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())
	exec.State().Store().SetNested("items", []any{"a", "b"})

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	want := []any{"parenta", "parentb"}
	if len(got.([]any)) != len(want) || got.([]any)[0] != want[0] || got.([]any)[1] != want[1] {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
}

func TestExecuteStepsForeachUnreadLocalAsyncFailureDoesNotFailIteration(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "foreach_unread_async",
		Nodes: []runtime.FlowNode{{
			ID:   "__foreach_1",
			Kind: runtime.FlowNodeForeach,
			Foreach: &runtime.ForeachBlock{
				Expr:    "items",
				ItemVar: "item",
				Steps:   []runtime.Step{{ID: "prefetch", Async: true, Body: `raise("ASYNC_FAIL", "boom")`}},
				Collects: []runtime.ForeachCollect{{
					Expr:  "item",
					Alias: "values",
				}},
			},
		}},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())
	exec.State().Store().SetNested("items", []any{"a"})

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	if got.([]any)[0] != "a" {
		t.Fatalf("values = %#v, want [a]", got)
	}
}

func TestExecuteStepsForeachReadLocalAsyncFailureFailsIteration(t *testing.T) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	stepExecutor := NewStepExecutor()
	executor := runtime.NewExecutor(NewExpressionEvaluator(), stepExecutor, NewLocalStepRunner(stepExecutor))
	flow := &runtime.Flow{
		ID: "foreach_read_async_failure",
		Nodes: []runtime.FlowNode{{
			ID:   "__foreach_1",
			Kind: runtime.FlowNodeForeach,
			Foreach: &runtime.ForeachBlock{
				Expr:     "items",
				ItemVar:  "item",
				Steps:    []runtime.Step{{ID: "prefetch", Async: true, Body: `raise("ASYNC_FAIL", "boom")`}},
				Collects: []runtime.ForeachCollect{{Expr: "prefetch.value", Alias: "values"}},
			},
		}},
	}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewValueStore())
	exec.State().Store().SetNested("items", []any{"a"})

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != "ASYNC_FAIL" || fe.Step != "values" || fe.AwaitedFrom != "prefetch" || fe.Meta["foreach"] != "__foreach_1" || fe.Meta["iteration"] != 0 {
		t.Fatalf("unexpected async failure: %#v", fe)
	}
	got, _ := exec.State().Store().Get("values")
	if got.([]any)[0] != nil {
		t.Fatalf("values = %#v, want [nil]", got)
	}
}
