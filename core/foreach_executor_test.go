package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type scriptedExpressionEvaluator struct {
	eval func(execution *Execution, expr string, program ExpressionProgram, storeKeys []string, extra map[string]any) (any, error)
}

func (e scriptedExpressionEvaluator) EvalExpression(execution *Execution, expr string, program ExpressionProgram, storeKeys []string, extra map[string]any) (any, error) {
	if e.eval == nil {
		return nil, nil
	}
	return e.eval(execution, expr, program, storeKeys, extra)
}

func newForeachExecutorHarness(t *testing.T, flow *Flow, evaluator ExpressionEvaluator, stepExecutor StepExecutor) (*Execution, *Executor) {
	t.Helper()
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())
	executor := NewExecutor(evaluator, stepExecutor, newIsolatedTestStepRunner(stepExecutor))
	return exec, executor
}

func TestExecuteSteps_ForeachCollectsOrderedValuesAndDoesNotLeakBodyResults(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		row := item.(map[string]any)
		execution.State().Store().SetNested(step.ID, map[string]any{"value": row["value"]})
		return "", nil
	}}
	flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:    "items",
			ItemVar: "item",
			Steps:   []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{
				Expr:  "normalize.value",
				Alias: "values",
			}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)
	exec.State().Store().SetNested("items", []any{
		map[string]any{"value": "a"},
		map[string]any{"value": "b"},
	})

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("values = %#v, want [a b]", got)
	}
	if leaked, ok := exec.State().Store().Get("normalize.value"); ok {
		t.Fatalf("foreach body result leaked to parent: %#v", leaked)
	}
}

func TestExecuteSteps_ForeachEmptyAndNoCollect(t *testing.T) {
	t.Run("empty input creates empty collect arrays", func(t *testing.T) {
		evaluator := scriptedExpressionEvaluator{eval: func(*Execution, string, ExpressionProgram, []string, map[string]any) (any, error) {
			return []any{}, nil
		}}
		flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
			ID:   "__foreach_1",
			Kind: FlowNodeForeach,
			Foreach: &ForeachBlock{
				Expr:     "items",
				ItemVar:  "item",
				Collects: []ForeachCollect{{Expr: "item", Alias: "items_seen"}},
			},
		}}}
		exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})

		if err := executor.ExecuteSteps(exec); err != nil {
			t.Fatalf("ExecuteSteps() error = %v", err)
		}
		got, ok := exec.State().Store().Get("items_seen")
		if !ok || !reflect.DeepEqual(got, []any{}) {
			t.Fatalf("items_seen = %#v present=%v, want empty slice", got, ok)
		}
	})

	t.Run("no collect writes nothing", func(t *testing.T) {
		evaluator := scriptedExpressionEvaluator{eval: func(*Execution, string, ExpressionProgram, []string, map[string]any) (any, error) {
			return []any{"a"}, nil
		}}
		flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
			ID:      "__foreach_1",
			Kind:    FlowNodeForeach,
			Foreach: &ForeachBlock{Expr: "items", ItemVar: "item"},
		}}}
		exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})

		if err := executor.ExecuteSteps(exec); err != nil {
			t.Fatalf("ExecuteSteps() error = %v", err)
		}
		if got := exec.State().Store().Snapshot(); len(got) != 0 {
			t.Fatalf("expected no parent writes, got %#v", got)
		}
	})
}

func TestExecuteSteps_ForeachRejectsNilAndNonArraySources(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "string", value: "not-array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := scriptedExpressionEvaluator{eval: func(*Execution, string, ExpressionProgram, []string, map[string]any) (any, error) {
				return tc.value, nil
			}}
			flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
				ID:   "__foreach_1",
				Kind: FlowNodeForeach,
				Foreach: &ForeachBlock{
					Expr:     "items",
					ItemVar:  "item",
					Collects: []ForeachCollect{{Expr: "item", Alias: "items_seen"}},
				},
			}}}
			exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})

			err := executor.ExecuteSteps(exec)
			fe, ok := err.(*FlowError)
			if !ok || fe.Step != "__foreach_1" || fe.Code != string(ErrorCodeRuntimeError) {
				t.Fatalf("unexpected error: %#v", err)
			}
			got, ok := exec.State().Store().Get("items_seen")
			if !ok || !reflect.DeepEqual(got, []any{}) {
				t.Fatalf("source failure should commit empty collect array, got %#v present=%v", got, ok)
			}
		})
	}
}

func TestExecuteSteps_ForeachBatchModeCollectsOneSlotPerBatch(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{1, 2, 3, 4, 5}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		batch, _ := execution.State().Store().Get("batch")
		execution.State().Store().SetNested(step.ID, map[string]any{"count": reflect.ValueOf(batch).Len()})
		return "", nil
	}}
	flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:      "items",
			ItemVar:   "batch",
			BatchSize: 2,
			Steps:     []Step{{ID: "count_batch"}},
			Collects:  []ForeachCollect{{Expr: "count_batch.count", Alias: "counts"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("counts")
	if !reflect.DeepEqual(got, []any{2, 2, 1}) {
		t.Fatalf("counts = %#v, want [2 2 1]", got)
	}
}

func TestExecuteSteps_ForeachTracingSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []any{"a", "b"}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureFailFast},
			Collects: []ForeachCollect{{Expr: "item", Alias: "items_seen"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})
	exec.Container.SetTracer(provider.Tracer("test"))

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}

	spans := recorder.Ended()
	parent := requireSpanNamed(t, spans, "foreach __foreach_1")
	assertSpanAttrString(t, parent, "foreach.id", "__foreach_1")
	assertSpanAttrBool(t, parent, "foreach.parallel", true)
	assertSpanAttrInt(t, parent, "parallel.max_in_flight", 2)
	assertSpanAttrString(t, parent, "parallel.on_failure", string(OnFailureFailFast))

	for i := 0; i < 2; i++ {
		child := requireSpanNamed(t, spans, fmt.Sprintf("foreach iteration %d", i))
		if child.Parent().SpanID() != parent.SpanContext().SpanID() {
			t.Fatalf("iteration span %d parent = %s, want %s", i, child.Parent().SpanID(), parent.SpanContext().SpanID())
		}
		assertSpanAttrString(t, child, "foreach.id", "__foreach_1")
		assertSpanAttrInt(t, child, "iteration", i)
		assertSpanAttrBool(t, child, "foreach.parallel", true)
		assertSpanAttrInt(t, child, "parallel.max_in_flight", 2)
		assertSpanAttrString(t, child, "parallel.on_failure", string(OnFailureFailFast))
	}
}

func TestExecuteSteps_ForeachFailureCommitsPartialCollectsAndMetadata(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []any{"ok", "fail", "not-run"}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		if item == "fail" {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "BOOM", Message: "failed", Step: step.ID}
		}
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}
	flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != "BOOM" || fe.Step != "normalize" || fe.Meta["foreach"] != "__foreach_1" || fe.Meta["iteration"] != 1 {
		t.Fatalf("unexpected foreach failure: %#v", fe)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{"ok", nil, nil}) {
		t.Fatalf("values = %#v, want [ok <nil> <nil>]", got)
	}
}

func TestExecuteSteps_ForeachSourceAndCollectErrorsAreAttributed(t *testing.T) {
	t.Run("source expression", func(t *testing.T) {
		evaluator := scriptedExpressionEvaluator{eval: func(*Execution, string, ExpressionProgram, []string, map[string]any) (any, error) {
			return nil, errors.New("bad source")
		}}
		flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
			ID:   "__foreach_1",
			Kind: FlowNodeForeach,
			Foreach: &ForeachBlock{
				Expr:     "items",
				ItemVar:  "item",
				Collects: []ForeachCollect{{Expr: "item", Alias: "items_seen"}},
			},
		}}}
		exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})

		err := executor.ExecuteSteps(exec)
		fe, ok := err.(*FlowError)
		if !ok || fe.Step != "__foreach_1" || fe.Meta["foreach"] != "__foreach_1" {
			t.Fatalf("unexpected source error: %#v", err)
		}
		got, _ := exec.State().Store().Get("items_seen")
		if !reflect.DeepEqual(got, []any{}) {
			t.Fatalf("items_seen = %#v, want empty slice", got)
		}
	})

	t.Run("collect expression", func(t *testing.T) {
		evaluator := scriptedExpressionEvaluator{eval: func(_ *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
			if expr == "items" {
				return []any{"a"}, nil
			}
			return nil, errors.New("bad collect")
		}}
		flow := &Flow{ID: "foreach", Nodes: []FlowNode{{
			ID:   "__foreach_1",
			Kind: FlowNodeForeach,
			Foreach: &ForeachBlock{
				Expr:     "items",
				ItemVar:  "item",
				Collects: []ForeachCollect{{Expr: "explode", Alias: "items_seen"}},
			},
		}}}
		exec, executor := newForeachExecutorHarness(t, flow, evaluator, &scriptedStepExecutor{})

		err := executor.ExecuteSteps(exec)
		fe, ok := err.(*FlowError)
		if !ok {
			t.Fatalf("expected FlowError, got %T", err)
		}
		if fe.Step != "items_seen" || fe.Meta["foreach"] != "__foreach_1" || fe.Meta["iteration"] != 0 || fe.Meta["collect"] != "items_seen" {
			t.Fatalf("unexpected collect error: %#v", fe)
		}
		got, _ := exec.State().Store().Get("items_seen")
		if !reflect.DeepEqual(got, []any{nil}) {
			t.Fatalf("items_seen = %#v, want [nil]", got)
		}
	})
}

func TestExecuteSteps_ForeachPassesCompiledExpressionMetadata(t *testing.T) {
	type call struct {
		expr      string
		program   ExpressionProgram
		storeKeys []string
	}
	var calls []call
	sourceProgram := struct{ name string }{"source"}
	collectProgram := struct{ name string }{"collect"}
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, program ExpressionProgram, storeKeys []string, _ map[string]any) (any, error) {
		calls = append(calls, call{expr: expr, program: program, storeKeys: append([]string{}, storeKeys...)})
		if expr == "items" {
			return []any{"a"}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		execution.State().Store().SetNested(step.ID, map[string]any{"value": "a"})
		return "", nil
	}}
	flow := &Flow{
		ID:      "foreach",
		DSLMode: DSLExecutionModeCompiled,
		Nodes: []FlowNode{{
			ID:   "__foreach_1",
			Kind: FlowNodeForeach,
			Foreach: &ForeachBlock{
				Expr:            "items",
				ExprStoreKeys:   []string{"request"},
				ExprProgram:     sourceProgram,
				ParentStoreKeys: []string{"request"},
				ItemVar:         "item",
				Steps:           []Step{{ID: "normalize", StoreKeys: []string{"item"}}},
				Collects: []ForeachCollect{{
					Expr:        "normalize.value",
					Alias:       "values",
					StoreKeys:   []string{"normalize"},
					ExprProgram: collectProgram,
				}},
			},
		}},
	}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want source and collect", calls)
	}
	if calls[0].expr != "items" || calls[0].program != sourceProgram || !reflect.DeepEqual(calls[0].storeKeys, []string{"request"}) {
		t.Fatalf("unexpected source call: %#v", calls[0])
	}
	if calls[1].expr != "normalize.value" || calls[1].program != collectProgram || !reflect.DeepEqual(calls[1].storeKeys, []string{"normalize"}) {
		t.Fatalf("unexpected collect call: %#v", calls[1])
	}
}

func TestNewIterationSourceUsesOriginalSliceAndBatchesWithoutPrecopy(t *testing.T) {
	input := []int{1, 2, 3}
	source, fe := newIterationSource(input, 0)
	if fe != nil {
		t.Fatalf("newIterationSource() error = %v", fe)
	}
	input[1] = 20
	if got := source.At(1); got != 20 {
		t.Fatalf("source.At(1) = %v, want updated original value 20", got)
	}

	batches, fe := newIterationSource(input, 2)
	if fe != nil {
		t.Fatalf("newIterationSource(batch) error = %v", fe)
	}
	first := batches.At(0).([]int)
	first[0] = 10
	if input[0] != 10 {
		t.Fatalf("batch did not expose original slice storage, input[0]=%d", input[0])
	}
	if batches.Len() != 2 {
		t.Fatalf("batches.Len() = %d, want 2", batches.Len())
	}
}

func TestEffectiveForeachOptionsUseForeachRuntimeDefaults(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	container.SetRuntimeConfig(RuntimeConfig{Parallel: ParallelConfig{
		ForeachDefaultMaxInFlight: 3,
		DefaultOnFailure:          OnFailureFailFast,
	}})
	exec := NewExecution(&Flow{ID: "foreach_options"}, container, nil, NewValueStore())

	got := effectiveForeachOptions(exec, ParallelOptions{})
	if got.MaxInFlight != 3 || got.OnFailure != OnFailureFailFast {
		t.Fatalf("effectiveForeachOptions() = %#v, want max_in_flight 3 and fail_fast", got)
	}
}

func TestExecuteSteps_ParallelForeachMaxInFlightLimitsActiveIterations(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}

	started := make(chan int, 4)
	release := make(chan struct{})
	var mu sync.Mutex
	active := 0
	maxActive := 0
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		started <- item.(int)
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		mu.Lock()
		active--
		mu.Unlock()
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}

	flow := &Flow{ID: "parallel_foreach_limit", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 2},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	done := make(chan error, 1)
	go func() { done <- executor.ExecuteSteps(exec) }()
	<-started
	<-started
	select {
	case third := <-started:
		t.Fatalf("third iteration started before a worker was released: %d", third)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	if maxActive > 2 {
		t.Fatalf("max active iterations = %d, want <= 2", maxActive)
	}
}

func TestExecuteSteps_ParallelForeachUsesAsyncRuntimeBudget(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}

	started := make(chan int, 4)
	release := make(chan struct{})
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		started <- item.(int)
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}

	flow := &Flow{ID: "parallel_foreach_runtime_budget", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 4},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)
	exec.Container.SetRuntimeConfig(RuntimeConfig{Async: AsyncConfig{RuntimeMaxInFlight: 1}})

	done := make(chan error, 1)
	go func() { done <- executor.ExecuteSteps(exec) }()
	<-started
	select {
	case second := <-started:
		t.Fatalf("second iteration started before async runtime budget was released: %d", second)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{0, 1, 2, 3}) {
		t.Fatalf("values = %#v, want [0 1 2 3]", got)
	}
}

func TestExecuteSteps_ParallelForeachCancellationReturnsFlowError(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}

	started := make(chan struct{})
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}

	flow := &Flow{ID: "parallel_foreach_cancel", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 1},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)
	ctx, cancel := context.WithCancel(context.Background())
	exec = exec.WithContext(ctx)

	done := make(chan error, 1)
	go func() { done <- executor.ExecuteSteps(exec) }()
	<-started
	cancel()

	err := <-done
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != string(ErrorCodeContextCancelled) || fe.Step != "__foreach_1" || fe.Meta["foreach"] != "__foreach_1" {
		t.Fatalf("unexpected cancellation error: %#v", fe)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{nil, nil, nil}) {
		t.Fatalf("values = %#v, want all nil collect slots", got)
	}
}

func TestExecuteSteps_ParallelForeachCollectsInInputOrder(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		time.Sleep(time.Duration(2-item.(int)) * 10 * time.Millisecond)
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}
	flow := &Flow{ID: "parallel_foreach_order", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 3},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{0, 1, 2}) {
		t.Fatalf("values = %#v, want [0 1 2]", got)
	}
}

func TestExecuteSteps_ParallelForeachWaitAllFailureOrdering(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		if item == 1 || item == 3 {
			time.Sleep(time.Duration(3-item.(int)) * 10 * time.Millisecond)
			return "", &FlowError{Type: ErrorTypePermanent, Code: fmt.Sprintf("BOOM_%d", item), Message: "failed", Step: step.ID}
		}
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}
	flow := &Flow{ID: "parallel_foreach_failures", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 4, OnFailure: OnFailureWaitAll},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != string(ErrorCodeParallelFailure) || len(fe.Failures) != 2 {
		t.Fatalf("unexpected grouped failure: %#v", fe)
	}
	if *fe.Failures[0].Iteration != 1 || fe.Failures[0].Error.Code != "BOOM_1" || *fe.Failures[1].Iteration != 3 || fe.Failures[1].Error.Code != "BOOM_3" {
		t.Fatalf("failures not in input order: %#v", fe.Failures)
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{0, nil, 2, nil}) {
		t.Fatalf("values = %#v, want [0 <nil> 2 <nil>]", got)
	}
}

func TestExecuteSteps_ParallelForeachWaitAllSingleFailureReturnsOriginal(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		if item == 1 {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "BOOM", Message: "failed", Step: step.ID}
		}
		execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
		return "", nil
	}}
	flow := &Flow{ID: "parallel_foreach_single_failure", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureWaitAll},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != "BOOM" || fe.Meta["foreach"] != "__foreach_1" || fe.Meta["iteration"] != 1 {
		t.Fatalf("unexpected single failure: %#v", fe)
	}
}

func TestExecuteSteps_ParallelForeachWaitAllOnErrorCanReadPartialCollects(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			item, _ := execution.State().Store().Get("item")
			if item == 1 || item == 3 {
				return "", &FlowError{Type: ErrorTypePermanent, Code: fmt.Sprintf("BOOM_%d", item), Message: "failed", Step: step.ID}
			}
			execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
			return "", nil
		},
		runOnError: func(execution *Execution, body string, fe *FlowError) error {
			values, _ := execution.State().Store().Get("values")
			execution.State().Store().SetNested("recovery", map[string]any{"values": values, "code": fe.Code})
			execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			return nil
		},
	}
	flow := &Flow{ID: "parallel_foreach_wait_all_on_error", OnErrorBody: "recover", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 4, OnFailure: OnFailureWaitAll},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected on_error to handle wait_all failure, got %v", err)
	}
	got, _ := exec.State().Store().Get("recovery.values")
	if !reflect.DeepEqual(got, []any{0, nil, 2, nil}) {
		t.Fatalf("recovery.values = %#v, want [0 <nil> 2 <nil>]", got)
	}
	if code, _ := exec.State().Store().Get("recovery.code"); code != string(ErrorCodeParallelFailure) {
		t.Fatalf("recovery.code = %#v, want PARALLEL_FAILURE", code)
	}
}

func TestExecuteSteps_ParallelForeachFailFastCancelsAndCommitsPartialCollects(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}

	itemZeroStarted := make(chan struct{})
	itemOneStarted := make(chan struct{})
	itemZeroCancelled := make(chan struct{})
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		switch item {
		case 0:
			close(itemZeroStarted)
			<-itemOneStarted
			<-ctx.Done()
			close(itemZeroCancelled)
			return "", ctx.Err()
		case 1:
			close(itemOneStarted)
			<-itemZeroStarted
			return "", &FlowError{Type: ErrorTypePermanent, Code: "BOOM", Message: "failed", Step: step.ID}
		default:
			execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
			return "", nil
		}
	}}
	flow := &Flow{ID: "parallel_foreach_fail_fast", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureFailFast},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != "BOOM" || fe.Meta["foreach"] != "__foreach_1" || fe.Meta["iteration"] != 1 {
		t.Fatalf("unexpected fail-fast error: %#v", fe)
	}
	select {
	case <-itemZeroCancelled:
	default:
		t.Fatal("in-flight iteration was not cancelled")
	}
	got, _ := exec.State().Store().Get("values")
	if !reflect.DeepEqual(got, []any{nil, nil, nil, nil}) {
		t.Fatalf("values = %#v, want all nil partial collect slots", got)
	}
}

func TestExecuteSteps_ParallelForeachFailFastOnErrorCanReadPartialCollects(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(execution *Execution, expr string, _ ExpressionProgram, _ []string, _ map[string]any) (any, error) {
		if expr == "items" {
			return []int{0, 1, 2, 3}, nil
		}
		value, ok := execution.State().Store().Get(expr)
		if !ok {
			return nil, fmt.Errorf("missing expr %s", expr)
		}
		return value, nil
	}}
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			item, _ := execution.State().Store().Get("item")
			if item == 1 {
				return "", &FlowError{Type: ErrorTypePermanent, Code: "BOOM", Message: "failed", Step: step.ID}
			}
			execution.State().Store().SetNested(step.ID, map[string]any{"value": item})
			return "", nil
		},
		runOnError: func(execution *Execution, body string, fe *FlowError) error {
			values, _ := execution.State().Store().Get("values")
			execution.State().Store().SetNested("recovery", map[string]any{"values": values, "code": fe.Code})
			execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			return nil
		},
	}
	flow := &Flow{ID: "parallel_foreach_fail_fast_on_error", OnErrorBody: "recover", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 1, OnFailure: OnFailureFailFast},
			Steps:    []Step{{ID: "normalize"}},
			Collects: []ForeachCollect{{Expr: "normalize.value", Alias: "values"}},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected on_error to handle fail_fast failure, got %v", err)
	}
	got, _ := exec.State().Store().Get("recovery.values")
	if !reflect.DeepEqual(got, []any{0, nil, nil, nil}) {
		t.Fatalf("recovery.values = %#v, want [0 <nil> <nil> <nil>]", got)
	}
	if code, _ := exec.State().Store().Get("recovery.code"); code != "BOOM" {
		t.Fatalf("recovery.code = %#v, want BOOM", code)
	}
}

func TestExecuteSteps_ParallelForeachFailFastCancelsLocalAsyncTasks(t *testing.T) {
	evaluator := scriptedExpressionEvaluator{eval: func(*Execution, string, ExpressionProgram, []string, map[string]any) (any, error) {
		return []int{0, 1}, nil
	}}

	itemZeroGateStarted := make(chan struct{})
	itemOneGateStarted := make(chan struct{})
	asyncStarted := make(chan struct{})
	asyncCancelled := make(chan struct{})
	var closeAsyncStarted sync.Once
	var closeAsyncCancelled sync.Once
	stepExecutor := &scriptedStepExecutor{runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
		item, _ := execution.State().Store().Get("item")
		switch step.ID {
		case "prefetch":
			if item == 0 {
				closeAsyncStarted.Do(func() { close(asyncStarted) })
				<-ctx.Done()
				closeAsyncCancelled.Do(func() { close(asyncCancelled) })
				return "", ctx.Err()
			}
			return "", nil
		case "gate":
			switch item {
			case 0:
				close(itemZeroGateStarted)
				<-itemOneGateStarted
				<-ctx.Done()
				return "", ctx.Err()
			case 1:
				close(itemOneGateStarted)
				<-itemZeroGateStarted
				<-asyncStarted
				return "", &FlowError{Type: ErrorTypePermanent, Code: "BOOM", Message: "failed", Step: step.ID}
			}
		}
		return "", nil
	}}
	flow := &Flow{ID: "parallel_foreach_fail_fast_async", Nodes: []FlowNode{{
		ID:   "__foreach_1",
		Kind: FlowNodeForeach,
		Foreach: &ForeachBlock{
			Expr:     "items",
			ItemVar:  "item",
			Parallel: true,
			Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureFailFast},
			Steps: []Step{
				{ID: "prefetch", Async: true},
				{ID: "gate"},
			},
		},
	}}}
	exec, executor := newForeachExecutorHarness(t, flow, evaluator, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != "BOOM" || fe.Meta["iteration"] != 1 {
		t.Fatalf("unexpected fail-fast async error: %#v", fe)
	}
	select {
	case <-asyncCancelled:
	default:
		t.Fatal("local async task in cancelled iteration was not cancelled")
	}
	if _, ok := exec.AsyncTasks().Get("prefetch"); ok {
		t.Fatal("local foreach async task leaked to parent scope")
	}
}

func requireSpanNamed(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name())
	}
	t.Fatalf("missing span %q in %#v", name, names)
	return nil
}

func assertSpanAttrString(t *testing.T, span sdktrace.ReadOnlySpan, key string, want string) {
	t.Helper()
	value := requireSpanAttr(t, span, key)
	if got := value.Value.AsString(); got != want {
		t.Fatalf("%s attr %s = %q, want %q", span.Name(), key, got, want)
	}
}

func assertSpanAttrBool(t *testing.T, span sdktrace.ReadOnlySpan, key string, want bool) {
	t.Helper()
	value := requireSpanAttr(t, span, key)
	if got := value.Value.AsBool(); got != want {
		t.Fatalf("%s attr %s = %v, want %v", span.Name(), key, got, want)
	}
}

func assertSpanAttrInt(t *testing.T, span sdktrace.ReadOnlySpan, key string, want int) {
	t.Helper()
	value := requireSpanAttr(t, span, key)
	if got := value.Value.AsInt64(); got != int64(want) {
		t.Fatalf("%s attr %s = %d, want %d", span.Name(), key, got, want)
	}
}

func requireSpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string) attribute.KeyValue {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr
		}
	}
	t.Fatalf("%s missing attr %s", span.Name(), key)
	return attribute.KeyValue{}
}
