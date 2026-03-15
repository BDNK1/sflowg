package dsl

import (
	"context"
	"testing"

	"github.com/BDNK1/sflowg/runtime"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newDSLTestMetrics(t *testing.T, decls map[string]runtime.UserMetricDecl) (*runtime.Metrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	metrics, err := runtime.NewTestMetricsWithReader(reader, decls)
	if err != nil {
		t.Fatalf("NewTestMetricsWithReader failed: %v", err)
	}
	return metrics, reader
}

func TestDSLMetricCounter_DefaultValue(t *testing.T) {
	metrics, reader := newDSLTestMetrics(t, nil)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	// metric.counter("test_counter") — should default to value 1.
	_, err := interp.Eval(context.Background(), `metric.counter("test_counter")`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.test_counter" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected sflowg.user.test_counter to be emitted")
	}
}

func TestDSLMetricCounter_WithValueAndLabels(t *testing.T) {
	metrics, reader := newDSLTestMetrics(t, nil)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	_, err := interp.Eval(context.Background(), `metric.counter("checkout", 5, {"provider": "stripe"})`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.checkout" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected sflowg.user.checkout to be emitted")
	}
}

func TestDSLMetricHistogram(t *testing.T) {
	metrics, reader := newDSLTestMetrics(t, nil)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	_, err := interp.Eval(context.Background(), `metric.histogram("latency_ms", 127)`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.latency_ms" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected sflowg.user.latency_ms to be emitted")
	}
}

func TestDSLMetricGauge(t *testing.T) {
	metrics, reader := newDSLTestMetrics(t, nil)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	_, err := interp.Eval(context.Background(), `metric.gauge("queue_depth", 42)`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.queue_depth" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected sflowg.user.queue_depth to be emitted")
	}
}

func TestDSLMetricInvalidCall_DoesNotFail(t *testing.T) {
	metrics, _ := newDSLTestMetrics(t, nil)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	// Invalid call (no args) should not panic or fail the step.
	_, err := interp.Eval(context.Background(), `metric.counter()`, globals)
	if err != nil {
		t.Fatalf("expected invalid metric call to not fail step, got %v", err)
	}

	// Invalid type for value should not panic.
	_, err = interp.Eval(context.Background(), `metric.histogram("x", "not_a_number")`, globals)
	if err != nil {
		t.Fatalf("expected invalid metric value to not fail step, got %v", err)
	}
}

func TestDSLPredeclaredHandle_Counter(t *testing.T) {
	decls := map[string]runtime.UserMetricDecl{
		"checkout_attempts": {
			Type: "counter",
			Labels: map[string]runtime.UserMetricLabel{
				"provider": {Type: "enum", Values: []string{"stripe", "paypal"}},
			},
		},
	}
	metrics, reader := newDSLTestMetrics(t, decls)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &runtime.Flow{ID: "test_flow"}
	exec := runtime.NewExecution(flow, container, nil, runtime.NewFlatValueStore())

	interp := &Interpreter{}
	globals := make(map[string]any)

	metricGlobals := BuildMetricGlobals(&exec)
	for k, v := range metricGlobals {
		globals[k] = v
	}

	_, err := interp.Eval(context.Background(), `metric.checkout_attempts.inc(1, {"provider": "stripe"})`, globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.checkout_attempts" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected sflowg.user.checkout_attempts to be emitted via predeclared handle")
	}
}
