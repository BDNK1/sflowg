package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type failingMetricStepExecutor struct {
	err error
}

func (s failingMetricStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	return "", s.err
}

type timeoutMetricStepExecutor struct{}

func (timeoutMetricStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type retryMetricStepExecutor struct {
	calls int
}

func (s *retryMetricStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	s.calls++
	if s.calls == 1 {
		return "", &FlowError{
			Type:    ErrorTypeTransient,
			Code:    "TEMPORARY",
			Message: "retry me",
			Step:    step.ID,
		}
	}
	return "", nil
}

type fallbackMetricStepExecutor struct{}

func (fallbackMetricStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	if step.Body == "fallback" {
		return "", nil
	}
	return "", &FlowError{
		Type:    ErrorTypePermanent,
		Code:    "PRIMARY_FAILED",
		Message: "primary failed",
		Step:    step.ID,
	}
}

type metricPlugin struct{}

func (metricPlugin) Charge(exec *Execution, args map[string]any) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

func TestValidateObservabilityConfig_RequiresMetricsEndpointWhenEnabled(t *testing.T) {
	err := ValidateObservabilityConfig(ObservabilityConfig{
		Metrics: MetricsConfig{Enabled: true},
	})
	if err == nil {
		t.Fatal("expected metrics validation error")
	}
	if !strings.Contains(err.Error(), "Endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
}

func TestValidateObservabilityConfig_RejectsNonIncreasingMetricBuckets(t *testing.T) {
	err := ValidateObservabilityConfig(ObservabilityConfig{
		Metrics: MetricsConfig{
			HistogramBuckets: HistogramBuckets{
				FlowMS: []float64{10, 25, 25},
			},
		},
	})
	if err == nil {
		t.Fatal("expected bucket validation error")
	}
	if !strings.Contains(err.Error(), "FlowMS") {
		t.Fatalf("expected FlowMS validation error, got %v", err)
	}
}

func TestMetricClassificationHelpers(t *testing.T) {
	if got := classifyMetricOutcome(nil); got != metricOutcomeSuccess {
		t.Fatalf("expected success outcome, got %q", got)
	}
	if got := classifyMetricOutcome(context.DeadlineExceeded); got != metricOutcomeTimeout {
		t.Fatalf("expected timeout outcome for deadline, got %q", got)
	}
	if got := classifyMetricOutcome(&FlowError{Type: ErrorTypeTimeout, Code: string(ErrorCodeDeadlineExceeded)}); got != metricOutcomeTimeout {
		t.Fatalf("expected timeout outcome for flow error, got %q", got)
	}
	if got := classifyMetricOutcome(errors.New("boom")); got != metricOutcomeError {
		t.Fatalf("expected error outcome, got %q", got)
	}
	if got := classifyHTTPStatus(http.StatusCreated); got != metricStatusClass2xx {
		t.Fatalf("expected 2xx classification, got %q", got)
	}
	if got := classifyHTTPStatus(http.StatusBadRequest); got != metricStatusClass4xx {
		t.Fatalf("expected 4xx classification, got %q", got)
	}
	if got := classifyHTTPStatus(http.StatusInternalServerError); got != metricStatusClass5xx {
		t.Fatalf("expected 5xx classification, got %q", got)
	}
}

func TestHandleRequest_EmitsSuccessMetrics(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	router := gin.New()
	flow := &Flow{
		ID: "payments",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "GET",
				"path":   "/payments",
			},
		},
		Steps: []Step{{ID: "charge"}},
	}
	executor := NewExecutor(noopEvaluator{}, noopStepExecutor{})
	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.flow.executions", map[string]string{
		"flow.id": "payments",
		"outcome": "success",
	}); got != 1 {
		t.Fatalf("expected flow execution count 1, got %d", got)
	}
	if got := findHistogramCount(t, rm, "sflowg.flow.duration_ms", map[string]string{
		"flow.id": "payments",
		"outcome": "success",
	}); got != 1 {
		t.Fatalf("expected flow duration count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
		"outcome": "success",
	}); got != 1 {
		t.Fatalf("expected step execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.http.server.requests", map[string]string{
		"flow.id":           "payments",
		"http.method":       "GET",
		"http.route":        "/payments",
		"http.status_class": "2xx",
	}); got != 1 {
		t.Fatalf("expected HTTP request count 1, got %d", got)
	}
}

func TestHandleRequest_EmitsErrorMetrics(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	router := gin.New()
	flow := &Flow{
		ID: "payments",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "GET",
				"path":   "/payments",
			},
		},
		Steps: []Step{{ID: "charge"}},
	}
	executor := NewExecutor(noopEvaluator{}, failingMetricStepExecutor{
		err: &FlowError{
			Type:    ErrorTypePermanent,
			Code:    "DECLINED",
			Message: "declined",
			Step:    "charge",
		},
	})
	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 status, got %d", rec.Code)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.flow.executions", map[string]string{
		"flow.id": "payments",
		"outcome": "error",
	}); got != 1 {
		t.Fatalf("expected error flow execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
		"outcome": "error",
	}); got != 1 {
		t.Fatalf("expected error step execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.http.server.requests", map[string]string{
		"flow.id":           "payments",
		"http.method":       "GET",
		"http.route":        "/payments",
		"http.status_class": "5xx",
	}); got != 1 {
		t.Fatalf("expected 5xx HTTP request count 1, got %d", got)
	}
}

func TestHandleRequest_EmitsTimeoutMetrics(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	router := gin.New()
	flow := &Flow{
		ID:      "payments",
		Timeout: 1,
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "GET",
				"path":   "/payments",
			},
		},
		Steps: []Step{{ID: "charge"}},
	}
	executor := NewExecutor(noopEvaluator{}, timeoutMetricStepExecutor{})
	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 status, got %d", rec.Code)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.flow.executions", map[string]string{
		"flow.id": "payments",
		"outcome": "timeout",
	}); got != 1 {
		t.Fatalf("expected timeout flow execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
		"outcome": "timeout",
	}); got != 1 {
		t.Fatalf("expected timeout step execution count 1, got %d", got)
	}
}

func TestHandleRequest_EmitsErrorFlowMetricWhenResponseDispatchFails(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	router := gin.New()
	flow := &Flow{
		ID: "payments",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "GET",
				"path":   "/payments",
			},
		},
		Steps: []Step{{ID: "respond", Type: "assign"}},
	}
	executor := NewExecutor(noopEvaluator{}, responseDescriptorStepExecutor{
		descriptor: &ResponseDescriptor{HandlerName: "missing.handler"},
	})
	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 status, got %d", rec.Code)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.flow.executions", map[string]string{
		"flow.id": "payments",
		"outcome": "error",
	}); got != 1 {
		t.Fatalf("expected error flow execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.http.server.requests", map[string]string{
		"flow.id":           "payments",
		"http.method":       "GET",
		"http.route":        "/payments",
		"http.status_class": "5xx",
	}); got != 1 {
		t.Fatalf("expected 5xx HTTP request count 1, got %d", got)
	}
}

func TestExecuteSteps_RecordsRetryMetrics(t *testing.T) {
	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{
		ID: "payments",
		Steps: []Step{{
			ID: "charge",
			Retry: &RetryConfig{
				MaxAttempts: 2,
			},
		}},
	}
	execution := NewExecution(flow, container, nil, newTestValueStore())
	stepExecutor := &retryMetricStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor)

	if err := executor.ExecuteSteps(&execution); err != nil {
		t.Fatalf("expected retrying step to succeed, got %v", err)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.step.retries", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
	}); got != 1 {
		t.Fatalf("expected retry count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
		"outcome": "success",
	}); got != 1 {
		t.Fatalf("expected successful step execution count 1, got %d", got)
	}
}

func TestExecuteSteps_RecordsFallbackMetrics(t *testing.T) {
	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	flow := &Flow{
		ID: "payments",
		Steps: []Step{{
			ID:           "charge",
			Body:         "primary",
			FallbackBody: "fallback",
		}},
	}
	execution := NewExecution(flow, container, nil, newTestValueStore())
	executor := NewExecutor(noopEvaluator{}, fallbackMetricStepExecutor{})

	if err := executor.ExecuteSteps(&execution); err != nil {
		t.Fatalf("expected fallback execution to succeed, got %v", err)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "primary",
		"outcome": "error",
	}); got != 1 {
		t.Fatalf("expected primary error execution count 1, got %d", got)
	}
	if got := findInt64SumValue(t, rm, "sflowg.step.executions", map[string]string{
		"flow.id": "payments",
		"step.id": "charge",
		"path":    "fallback",
		"outcome": "success",
	}); got != 1 {
		t.Fatalf("expected fallback success execution count 1, got %d", got)
	}
}

func TestContainerTask_RecordsPluginMetrics(t *testing.T) {
	metrics, reader := newTestMetrics(t)
	container := NewContainer(NewLogger(nil))
	container.SetMetrics(metrics)

	if err := container.RegisterPlugin("wallet", &metricPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	task := container.GetTask("wallet.charge")
	if task == nil {
		t.Fatal("expected registered plugin task")
	}

	execution := NewExecution(&Flow{ID: "payments"}, container, nil, newTestValueStore())
	var err error
	execution.WithActiveStep("charge", func() {
		_, err = task.Execute(&execution, map[string]any{})
	})
	if err != nil {
		t.Fatalf("expected plugin task to succeed, got %v", err)
	}

	rm := collectMetrics(t, reader)
	if got := findInt64SumValue(t, rm, "sflowg.plugin.calls", map[string]string{
		"flow.id":       "payments",
		"step.id":       "charge",
		"plugin.name":   "wallet",
		"plugin.method": "charge",
		"outcome":       "success",
	}); got != 1 {
		t.Fatalf("expected plugin call count 1, got %d", got)
	}
	if got := findHistogramCount(t, rm, "sflowg.plugin.duration_ms", map[string]string{
		"flow.id":       "payments",
		"step.id":       "charge",
		"plugin.name":   "wallet",
		"plugin.method": "charge",
		"outcome":       "success",
	}); got != 1 {
		t.Fatalf("expected plugin duration count 1, got %d", got)
	}
}

func newTestMetrics(t *testing.T) (*Metrics, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider, err := newMeterProvider(MetricsConfig{}, reader)
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
	return metrics, reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) *metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	return &rm
}

func findInt64SumValue(t *testing.T, rm *metricdata.ResourceMetrics, name string, attrs map[string]string) int64 {
	t.Helper()

	metric := findMetric(t, rm, name)
	sum, ok := metric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %s is %T, want metricdata.Sum[int64]", name, metric.Data)
	}
	for _, point := range sum.DataPoints {
		if attributesMatch(point.Attributes, attrs) {
			return point.Value
		}
	}
	t.Fatalf("metric %s missing data point for attrs %v", name, attrs)
	return 0
}

func findHistogramCount(t *testing.T, rm *metricdata.ResourceMetrics, name string, attrs map[string]string) uint64 {
	t.Helper()

	metric := findMetric(t, rm, name)
	histogram, ok := metric.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("metric %s is %T, want metricdata.Histogram[float64]", name, metric.Data)
	}
	for _, point := range histogram.DataPoints {
		if attributesMatch(point.Attributes, attrs) {
			return point.Count
		}
	}
	t.Fatalf("metric %s missing histogram point for attrs %v", name, attrs)
	return 0
}

func findMetric(t *testing.T, rm *metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()

	for _, scopeMetrics := range rm.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}
	t.Fatalf("metric %s not found", name)
	return metricdata.Metrics{}
}

func attributesMatch(set attribute.Set, want map[string]string) bool {
	if set.Len() != len(want) {
		return false
	}
	for key, wantValue := range want {
		gotValue, ok := set.Value(attribute.Key(key))
		if !ok || gotValue.AsString() != wantValue {
			return false
		}
	}
	return true
}
