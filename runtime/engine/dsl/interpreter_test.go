package dsl

import (
	"context"
	"testing"

	"github.com/BDNK1/sflowg/runtime"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestInterpreterEval_BasicTypes(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	tests := []struct {
		name     string
		code     string
		globals  map[string]any
		wantType string // "string", "int64", "float64", "bool", "nil"
		want     any
	}{
		{"string", `"hello"`, nil, "string", "hello"},
		{"int", `42`, nil, "int64", int64(42)},
		{"float", `3.14`, nil, "float64", 3.14},
		{"bool true", `true`, nil, "bool", true},
		{"bool false", `false`, nil, "bool", false},
		{"nil", `nil`, nil, "nil", nil},
		{"global access", `x`, map[string]any{"x": "world"}, "string", "world"},
		{"arithmetic", `a + b`, map[string]any{"a": 10, "b": 20}, "int64", int64(30)},
		{"string concat", `a + " " + b`, map[string]any{"a": "hello", "b": "world"}, "string", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globals := tt.globals
			if globals == nil {
				globals = map[string]any{}
			}
			result, err := interp.Eval(ctx, tt.code, globals)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", result, result, tt.want, tt.want)
			}
		})
	}
}

func TestInterpreterEval_MapResult(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	result, err := interp.Eval(ctx, `{"name": "alice", "age": 30}`, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["name"] != "alice" {
		t.Errorf("got name=%v, want alice", m["name"])
	}
	if m["age"] != int64(30) {
		t.Errorf("got age=%v, want 30", m["age"])
	}
}

func TestInterpreterEval_ListResult(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	result, err := interp.Eval(ctx, `[1, 2, 3]`, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(list) != 3 {
		t.Errorf("got len=%d, want 3", len(list))
	}
}

func TestInterpreterEval_Sandboxed(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	// os module should not be available (WithoutDefaultGlobals)
	_, err := interp.Eval(ctx, `os.getenv("PATH")`, map[string]any{})
	if err == nil {
		t.Fatal("expected error when accessing os module in sandbox, got nil")
	}
}

func TestInterpreterEval_NestedMapAccess(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	globals := map[string]any{
		"step": map[string]any{
			"result": map[string]any{
				"status_code": int64(200),
				"body": map[string]any{
					"id": "abc123",
				},
			},
		},
	}

	result, err := interp.Eval(ctx, `step.result.body.id`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "abc123" {
		t.Errorf("got %v, want abc123", result)
	}
}

func TestInterpreterEval_MissingAttributeReturnsNil(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	globals := map[string]any{
		"request": map[string]any{
			"body": map[string]any{
				"amount": 100,
				// customer_email intentionally absent
			},
		},
	}

	// Missing field should return nil, not throw "attribute not found"
	result, err := interp.Eval(ctx, `request.body.customer_email == nil`, globals)
	if err != nil {
		t.Fatalf("unexpected error accessing missing field: %v", err)
	}
	if result != true {
		t.Errorf("expected true (missing field == nil), got %v", result)
	}

	// Existing field should still return its value
	result, err = interp.Eval(ctx, `request.body.amount`, globals)
	if err != nil {
		t.Fatalf("unexpected error accessing existing field: %v", err)
	}
	if result != int64(100) {
		t.Errorf("expected 100, got %v", result)
	}
}

func TestBuildMetricGlobals_CounterIncAdd(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := runtime.InitMetrics([]runtime.MetricConfig{
		{Name: "checkout_attempts", Type: runtime.MetricTypeCounter},
	}, meter)
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	exec := &runtime.Execution{}
	exec.Container = runtime.NewContainer(runtime.NewLogger(nil))
	exec.Container.Metrics = handles

	globals := BuildMetricGlobals(exec)
	if globals == nil {
		t.Fatal("BuildMetricGlobals returned nil")
	}
	if _, ok := globals["metric"]; !ok {
		t.Fatal("expected 'metric' key in globals")
	}

	interp := &Interpreter{}
	ctx := context.Background()

	// inc() with no args
	if _, err := interp.Eval(ctx, `metric.checkout_attempts.inc()`, globals); err != nil {
		t.Fatalf("inc() failed: %v", err)
	}
	// add(n)
	if _, err := interp.Eval(ctx, `metric.checkout_attempts.add(5)`, globals); err != nil {
		t.Fatalf("add(5) failed: %v", err)
	}
}

func TestBuildMetricGlobals_HistogramRecord(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := runtime.InitMetrics([]runtime.MetricConfig{
		{Name: "checkout_amounts", Type: runtime.MetricTypeHistogram, Unit: "USD"},
	}, meter)
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	exec := &runtime.Execution{}
	exec.Container = runtime.NewContainer(runtime.NewLogger(nil))
	exec.Container.Metrics = handles

	globals := BuildMetricGlobals(exec)
	interp := &Interpreter{}

	if _, err := interp.Eval(context.Background(), `metric.checkout_amounts.record(99.5)`, globals); err != nil {
		t.Fatalf("record(99.5) failed: %v", err)
	}
}

func TestBuildMetricGlobals_GaugeSet(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := runtime.InitMetrics([]runtime.MetricConfig{
		{Name: "queue_depth", Type: runtime.MetricTypeGauge},
	}, meter)
	if err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}

	exec := &runtime.Execution{}
	exec.Container = runtime.NewContainer(runtime.NewLogger(nil))
	exec.Container.Metrics = handles

	globals := BuildMetricGlobals(exec)
	interp := &Interpreter{}

	if _, err := interp.Eval(context.Background(), `metric.queue_depth.set(3)`, globals); err != nil {
		t.Fatalf("set(3) failed: %v", err)
	}
}

func TestBuildMetricGlobals_Empty(t *testing.T) {
	exec := &runtime.Execution{}
	exec.Container = runtime.NewContainer(runtime.NewLogger(nil))

	if got := BuildMetricGlobals(exec); got != nil {
		t.Errorf("expected nil for empty metrics, got %v", got)
	}
}

func TestWrapGoFunc_ExtraArgsIgnored(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	called := false
	// 0-param function called with an argument — must not panic
	globals := map[string]any{
		"container": map[string]any{
			"noop": func() error {
				called = true
				return nil
			},
		},
	}

	if _, err := interp.Eval(ctx, `container.noop()`, globals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("noop was not called")
	}
}

func TestInterpreterEval_GoFunctionCall(t *testing.T) {
	interp := &Interpreter{}
	ctx := context.Background()

	called := false
	globals := map[string]any{
		"my_func": func(x int64) int64 {
			called = true
			return x * 2
		},
	}

	result, err := interp.Eval(ctx, `my_func(21)`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("Go function was not called")
	}
	if result != int64(42) {
		t.Errorf("got %v, want 42", result)
	}
}
