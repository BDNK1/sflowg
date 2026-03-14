package runtime

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type noopEvaluator struct{}

func (noopEvaluator) Eval(execution *Execution, expression string) (any, error) {
	return nil, nil
}

type noopStepExecutor struct{}

func (noopStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	return "", nil
}

type responseDescriptorStepExecutor struct {
	descriptor *ResponseDescriptor
}

func (s responseDescriptorStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	execution.ResponseDescriptor = s.descriptor
	return "", nil
}

type testValueStore struct {
	values map[string]any
}

func newTestValueStore() ValueStore {
	return &testValueStore{values: make(map[string]any)}
}

func (s *testValueStore) Set(key string, value any) {
	s.values[key] = value
}

func (s *testValueStore) Get(key string) (any, bool) {
	value, ok := s.values[key]
	return value, ok
}

func (s *testValueStore) SetNested(prefix string, value any) {
	s.values[prefix] = value
}

func (s *testValueStore) All() map[string]any {
	return s.values
}

func TestHandleRequest_LogsCompletedRequests(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	var buf bytes.Buffer
	logger := NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})

	container := NewContainer(NewLogger(logger))

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
	}
	executor := NewExecutor(noopEvaluator{}, noopStepExecutor{})

	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	output := buf.String()
	for _, want := range []string{
		`"msg":"HTTP request completed"`,
		`"method":"GET"`,
		`"path":"/payments"`,
		`"status_code":200`,
		`"flow_id":"payments"`,
		`"execution_id":"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected log output to contain %s, got %s", want, output)
		}
	}
}

func TestHandleRequest_ContinuesInboundTraceContext(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var buf bytes.Buffer
	logger := NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})

	container := NewContainer(NewLogger(logger))
	provider := sdktrace.NewTracerProvider()
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()
	container.SetTracer(provider.Tracer("test"))

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
	}
	executor := NewExecutor(noopEvaluator{}, noopStepExecutor{})

	NewHttpHandler(flow, container, executor, nil, newTestValueStore, router)

	req := httptest.NewRequest(http.MethodGet, "/payments", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	output := buf.String()
	if !strings.Contains(output, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`) {
		t.Fatalf("expected request log to include continued trace ID, got %s", output)
	}
}

func TestHandleRequest_MarksRootSpanErrorWhenResponseDispatchFails(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(recorder)
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	container := NewContainer(NewLogger(NewObservabilityLoggerWithWriter(&bytes.Buffer{}, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})))
	container.SetTracer(provider.Tracer("test"))

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

	flowSpan := findSpanByName(t, recorder.Ended(), "flow payments")
	if flowSpan.Status().Code != codes.Error {
		t.Fatalf("expected root span status=error, got %s", flowSpan.Status().Code)
	}
	if !strings.Contains(flowSpan.Status().Description, "unknown response handler") {
		t.Fatalf("expected root span error description, got %q", flowSpan.Status().Description)
	}
	if value, ok := spanAttribute(flowSpan.Attributes(), attribute.Key("http.status_code")); !ok || value.AsInt64() != http.StatusInternalServerError {
		t.Fatalf("expected root span http.status_code=500, got %v (present=%v)", value, ok)
	}
}

func findSpanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("expected to find span %q", name)
	return nil
}

func spanAttribute(attrs []attribute.KeyValue, key attribute.Key) (attribute.Value, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}
