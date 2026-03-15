package runtime

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestInitMetrics_CreatesCorrectTypes(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")

	handles, err := InitMetrics([]MetricConfig{
		{Name: "req_count", Type: MetricTypeCounter, Description: "requests"},
		{Name: "req_duration", Type: MetricTypeHistogram, Unit: "ms"},
		{Name: "active_conns", Type: MetricTypeGauge},
	}, meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(handles) != 3 {
		t.Fatalf("expected 3 handles, got %d", len(handles))
	}
	if handles["req_count"].Type() != MetricTypeCounter {
		t.Errorf("expected counter, got %s", handles["req_count"].Type())
	}
	if handles["req_duration"].Type() != MetricTypeHistogram {
		t.Errorf("expected histogram, got %s", handles["req_duration"].Type())
	}
	if handles["active_conns"].Type() != MetricTypeGauge {
		t.Errorf("expected gauge, got %s", handles["active_conns"].Type())
	}
}

func TestInitMetrics_EmptyConfig(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := InitMetrics(nil, meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(handles) != 0 {
		t.Errorf("expected empty handles, got %d", len(handles))
	}
}

func TestInitMetrics_UnknownType(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	_, err := InitMetrics([]MetricConfig{
		{Name: "bad", Type: "unknown"},
	}, meter)
	if err == nil {
		t.Fatal("expected error for unknown metric type, got nil")
	}
}

func TestMetricHandle_CounterOperations(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := InitMetrics([]MetricConfig{
		{Name: "hits", Type: MetricTypeCounter},
	}, meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	h := handles["hits"]

	// All calls should succeed with noop meter
	if err := h.Add(ctx, 1.0); err != nil {
		t.Errorf("Add returned error: %v", err)
	}
	if err := h.Add(ctx, 5.0); err != nil {
		t.Errorf("Add returned error: %v", err)
	}
	// Record and Set are no-ops on counter — must not error
	if err := h.Record(ctx, 1.0); err != nil {
		t.Errorf("Record on counter returned error: %v", err)
	}
	if err := h.Set(ctx, 1.0); err != nil {
		t.Errorf("Set on counter returned error: %v", err)
	}
}

func TestMetricHandle_HistogramOperations(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := InitMetrics([]MetricConfig{
		{Name: "latency", Type: MetricTypeHistogram, Unit: "ms"},
	}, meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	h := handles["latency"]

	if err := h.Record(ctx, 42.5); err != nil {
		t.Errorf("Record returned error: %v", err)
	}
}

func TestMetricHandle_GaugeOperations(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	handles, err := InitMetrics([]MetricConfig{
		{Name: "queue_depth", Type: MetricTypeGauge},
	}, meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	h := handles["queue_depth"]

	if err := h.Set(ctx, 7.0); err != nil {
		t.Errorf("Set returned error: %v", err)
	}
}
