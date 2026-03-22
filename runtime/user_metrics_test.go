package runtime

import (
	"context"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestValidateUserMetricsConfig_ValidDeclaration(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"checkout_attempts": {
				Type:        "counter",
				Unit:        "1",
				Description: "Checkout attempts",
				Labels: map[string]UserMetricLabel{
					"provider": {Type: "enum", Values: []string{"stripe", "paypal"}},
				},
			},
		},
	}
	if err := validateUserMetricsConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateUserMetricsConfig_AllPrimitiveTypes(t *testing.T) {
	for _, metricType := range []string{"counter", "updowncounter", "histogram", "gauge"} {
		cfg := UserMetricsConfig{
			Declarations: map[string]UserMetricDecl{
				"test_metric": {Type: metricType},
			},
		}
		if err := validateUserMetricsConfig(cfg); err != nil {
			t.Fatalf("expected type %q to be valid, got %v", metricType, err)
		}
	}
}

func TestValidateUserMetricsConfig_InvalidType(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"bad_metric": {Type: "timer"},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid type")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected 'invalid type' in error, got %v", err)
	}
}

func TestValidateUserMetricsConfig_BucketsOnNonHistogram(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"bad_metric": {
				Type:    "counter",
				Buckets: []float64{10, 50, 100},
			},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for buckets on non-histogram")
	}
	if !strings.Contains(err.Error(), "buckets") {
		t.Fatalf("expected 'buckets' in error, got %v", err)
	}
}

func TestValidateUserMetricsConfig_BucketsOnHistogramValid(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"latency": {
				Type:    "histogram",
				Buckets: []float64{10, 50, 100, 500},
			},
		},
	}
	if err := validateUserMetricsConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateUserMetricsConfig_NonIncreasingBuckets(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"latency": {
				Type:    "histogram",
				Buckets: []float64{100, 50, 200},
			},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for non-increasing buckets")
	}
}

func TestValidateUserMetricsConfig_InvalidLabelType(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"test": {
				Type: "counter",
				Labels: map[string]UserMetricLabel{
					"provider": {Type: "regex", Values: []string{".*"}},
				},
			},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid label type")
	}
	if !strings.Contains(err.Error(), "must be enum") {
		t.Fatalf("expected 'must be enum' in error, got %v", err)
	}
}

func TestValidateUserMetricsConfig_EmptyEnumValues(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"test": {
				Type: "counter",
				Labels: map[string]UserMetricLabel{
					"provider": {Type: "enum", Values: []string{}},
				},
			},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty enum values")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected 'at least one' in error, got %v", err)
	}
}

func TestValidateUserMetricsConfig_InvalidMetricName(t *testing.T) {
	cfg := UserMetricsConfig{
		Declarations: map[string]UserMetricDecl{
			"invalid name!": {Type: "counter"},
		},
	}
	err := validateUserMetricsConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid metric name")
	}
}

func TestValidateUserMetricsConfig_RejectsReservedDSLNames(t *testing.T) {
	for _, name := range []string{"counter", "updowncounter", "histogram", "gauge"} {
		cfg := UserMetricsConfig{
			Declarations: map[string]UserMetricDecl{
				name: {Type: "counter"},
			},
		}
		err := validateUserMetricsConfig(cfg)
		if err == nil {
			t.Fatalf("expected validation error for reserved DSL name %q", name)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("expected 'reserved' in error for name %q, got %v", name, err)
		}
	}
}

func TestValidateUserMetricContext_ValidScalars(t *testing.T) {
	ctx := map[string]any{
		"channel": "web",
		"region":  "eu-west-1",
	}
	if err := ValidateUserMetricContext(ctx); err != nil {
		t.Fatalf("expected valid context, got %v", err)
	}
}

func TestValidateUserMetricContext_RejectsNestedMap(t *testing.T) {
	ctx := map[string]any{
		"nested": map[string]any{"a": "b"},
	}
	err := ValidateUserMetricContext(ctx)
	if err == nil {
		t.Fatal("expected validation error for nested map")
	}
	if !strings.Contains(err.Error(), "scalar") {
		t.Fatalf("expected 'scalar' in error, got %v", err)
	}
}

func TestValidateUserMetricContext_RejectsArray(t *testing.T) {
	ctx := map[string]any{
		"list": []any{"a", "b"},
	}
	err := ValidateUserMetricContext(ctx)
	if err == nil {
		t.Fatal("expected validation error for array")
	}
}

// --- User metrics runtime tests ---

func newTestUserMetrics(t *testing.T, decls map[string]UserMetricDecl) (*Metrics, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	cfg := MetricsConfig{
		User: UserMetricsConfig{Declarations: decls},
	}
	provider, err := newMeterProvider(cfg, reader)
	if err != nil {
		t.Fatalf("newMeterProvider failed: %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	metrics, err := newMetrics(provider)
	if err != nil {
		t.Fatalf("newMetrics failed: %v", err)
	}

	if err := metrics.InitUserMetrics(decls); err != nil {
		t.Fatalf("InitUserMetrics failed: %v", err)
	}

	return metrics, reader
}

func TestRecordUserCounter_DynamicEmitsDatapoint(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	stepExec := execution.WithActivePath(SuccessPathPrimary).WithActiveStep("charge")
	metrics.RecordUserCounter(stepExec, "checkout_attempts", 1, map[string]any{
		"provider": "stripe",
	})

	rm := collectMetrics(t, reader)
	findFloat64SumValue(t, rm, "sflowg.user.checkout_attempts", map[string]string{
		"flow.id":  "payments",
		"step.id":  "charge",
		"path":     "primary",
		"provider": "stripe",
	})
}

func TestRecordUserCounter_DropsNegativeValue(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserCounter(execution, "bad_counter", -1, nil)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	// Should not find any user metric — the negative value was dropped.
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.bad_counter" {
				t.Fatal("expected negative counter metric to be dropped")
			}
		}
	}
}

func TestRecordUserCounter_DropsZeroValue(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserCounter(execution, "zero_counter", 0, nil)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.zero_counter" {
				t.Fatal("expected zero counter metric to be dropped")
			}
		}
	}
}

func TestRecordUserUpDownCounter_SupportsNegativeDelta(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserUpDownCounter(execution, "active_sessions", -1, nil)

	rm := collectMetrics(t, reader)
	got := findFloat64SumValue(t, rm, "sflowg.user.active_sessions", map[string]string{
		"flow.id": "payments",
	})
	if got != -1 {
		t.Fatalf("expected updowncounter value -1, got %f", got)
	}
}

func TestRecordUserHistogram_EmitsDatapoint(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserHistogram(execution, "payment_latency_ms", 127.5, nil)

	rm := collectMetrics(t, reader)
	findHistogramCount(t, rm, "sflowg.user.payment_latency_ms", map[string]string{
		"flow.id": "payments",
	})
}

func TestRecordUserGauge_EmitsDatapoint(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserGauge(execution, "queue_depth", 42, nil)

	rm := collectMetrics(t, reader)
	findFloat64GaugeValue(t, rm, "sflowg.user.queue_depth", map[string]string{
		"flow.id": "payments",
	})
}

func TestRecordUserCounter_DynamicTypeConflict(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	// First call creates a counter.
	metrics.RecordUserCounter(execution, "my_metric", 1, nil)
	// Second call with different type should be dropped.
	metrics.RecordUserHistogram(execution, "my_metric", 100, nil)

	rm := collectMetrics(t, reader)
	// Should find the counter but not a histogram.
	findFloat64SumValue(t, rm, "sflowg.user.my_metric", map[string]string{
		"flow.id": "payments",
	})
}

func TestRecordUserCounter_DropsReservedLabelKey(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	// Using reserved key "flow.id" in labels should drop the entire datapoint.
	metrics.RecordUserCounter(execution, "bad_labels", 1, map[string]any{
		"flow.id": "override",
	})

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "sflowg.user.bad_labels" {
				t.Fatal("expected metric with reserved label to be dropped")
			}
		}
	}
}

func TestRecordUserCounter_PredeclaredEnforcesEnum(t *testing.T) {
	decls := map[string]UserMetricDecl{
		"checkout_attempts": {
			Type: "counter",
			Labels: map[string]UserMetricLabel{
				"provider": {Type: "enum", Values: []string{"stripe", "paypal"}},
			},
		},
	}
	metrics, reader := newTestUserMetrics(t, decls)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	// Valid enum value should work.
	metrics.RecordUserCounter(execution, "checkout_attempts", 1, map[string]any{
		"provider": "stripe",
	})

	rm := collectMetrics(t, reader)
	findFloat64SumValue(t, rm, "sflowg.user.checkout_attempts", map[string]string{
		"flow.id":  "payments",
		"provider": "stripe",
	})
}

func TestRecordUserMetrics_AutoAttachesPropertyContext(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	metrics.SetUserMetricContext(map[string]string{
		"channel": "web",
		"region":  "eu-west-1",
	})

	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	metrics.RecordUserCounter(execution, "checkout_attempts", 1, nil)

	rm := collectMetrics(t, reader)
	findFloat64SumValue(t, rm, "sflowg.user.checkout_attempts", map[string]string{
		"flow.id": "payments",
		"channel": "web",
		"region":  "eu-west-1",
	})
}

func TestRecordUserMetrics_FallbackPath(t *testing.T) {
	metrics, reader := newTestUserMetrics(t, nil)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{ID: "payments"}
	execution := NewExecution(flow, container, nil, newTestValueStore())

	stepExec := execution.WithActivePath(SuccessPathFallback).WithActiveStep("charge")
	metrics.RecordUserCounter(stepExec, "checkout_attempts", 1, nil)

	rm := collectMetrics(t, reader)
	findFloat64SumValue(t, rm, "sflowg.user.checkout_attempts", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "fallback",
	})
}

func TestWithActivePath_ScopesAndRestores(t *testing.T) {
	exec := NewExecution(&Flow{ID: "test"}, nil, nil, newTestValueStore())

	if exec.ActivePath() != "" {
		t.Fatalf("expected empty path before scope, got %q", exec.ActivePath())
	}

	// WithActivePath returns a derived copy — the original is never mutated.
	scopedExec := exec.WithActivePath(SuccessPathPrimary)

	if scopedExec.ActivePath() != SuccessPathPrimary {
		t.Fatalf("expected primary path in scoped copy, got %q", scopedExec.ActivePath())
	}
	if exec.ActivePath() != "" {
		t.Fatalf("expected original to remain unmodified after WithActivePath, got %q", exec.ActivePath())
	}
}

// --- Test helpers ---

func findFloat64SumValue(t *testing.T, rm *metricdata.ResourceMetrics, name string, attrs map[string]string) float64 {
	t.Helper()

	metric := findMetric(t, rm, name)
	sum, ok := metric.Data.(metricdata.Sum[float64])
	if !ok {
		t.Fatalf("metric %s is %T, want metricdata.Sum[float64]", name, metric.Data)
	}
	for _, point := range sum.DataPoints {
		if attributesMatch(point.Attributes, attrs) {
			return point.Value
		}
	}
	t.Fatalf("metric %s missing data point for attrs %v", name, attrs)
	return 0
}

func findFloat64GaugeValue(t *testing.T, rm *metricdata.ResourceMetrics, name string, attrs map[string]string) float64 {
	t.Helper()

	metric := findMetric(t, rm, name)
	gauge, ok := metric.Data.(metricdata.Gauge[float64])
	if !ok {
		t.Fatalf("metric %s is %T, want metricdata.Gauge[float64]", name, metric.Data)
	}
	for _, point := range gauge.DataPoints {
		if attributesMatch(point.Attributes, attrs) {
			return point.Value
		}
	}
	t.Fatalf("metric %s missing gauge point for attrs %v", name, attrs)
	return 0
}
