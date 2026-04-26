package httptransport

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	runtime "github.com/BDNK1/sflowg/runtime"
	"github.com/BDNK1/sflowg/runtime/validation/httpinput"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type Execution = runtime.Execution
type Entrypoint = runtime.Entrypoint
type Flow = runtime.Flow
type FlowError = runtime.FlowError
type LoggingConfig = runtime.LoggingConfig
type ObservabilityConfig = runtime.ObservabilityConfig
type ResponseDescriptor = runtime.ResponseDescriptor
type Step = runtime.Step
type StepInput = runtime.StepInput
type StepOutput = runtime.StepOutput
type StepExecutor = runtime.StepExecutor
type StepRunner = runtime.StepRunner
type SuccessPath = runtime.SuccessPath
type ValueStore = runtime.ValueStore

var NewContainer = runtime.NewContainer
var NewExecution = runtime.NewExecution
var NewExecutor = runtime.NewExecutor
var NewLogger = runtime.NewLogger
var NewObservabilityLoggerWithWriter = runtime.NewObservabilityLoggerWithWriter
var NewRunState = runtime.NewRunState
var NewValueStore = runtime.NewValueStore

type noopEvaluator struct{}

func (noopEvaluator) Eval(execution *Execution, expression string) (any, error) {
	return nil, nil
}

type isolatedTestStepRunner struct {
	executor StepExecutor
}

func newIsolatedTestStepRunner(executor StepExecutor) StepRunner {
	return isolatedTestStepRunner{executor: executor}
}

func (r isolatedTestStepRunner) RunStep(ctx context.Context, execution *Execution, input StepInput) (StepOutput, error) {
	store := NewValueStore()
	for k, v := range input.Input {
		store.SetNested(k, v)
	}

	isolated := execution.WithIsolatedState(NewRunState(store))
	step := Step{
		ID:       input.StepID,
		Body:     input.Body,
		Timeout:  input.Timeout,
		Compiled: input.Compiled,
	}

	next, err := r.executor.ExecuteStep(ctx, isolated, step)
	if err != nil {
		return StepOutput{}, err
	}

	result, _ := isolated.State().Store().Get(input.StepID)
	return StepOutput{
		Result:   result,
		Response: isolated.State().Response(),
		Next:     next,
	}, nil
}

func mustParseHTTPBinding(t *testing.T, config map[string]any) *httpinput.Binding {
	t.Helper()
	binding, err := httpinput.ParseBinding(config)
	if err != nil {
		t.Fatalf("httpinput.ParseBinding failed: %v", err)
	}
	return binding
}

func (noopEvaluator) EvalWithEnv(execution *Execution, expression string, extraVars map[string]any) (any, error) {
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
	execution.State().SetResponse(s.descriptor)
	return "", nil
}

type captureInputStepExecutor struct {
	input map[string]any
	runs  int
}

func (s *captureInputStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	s.runs++
	s.input = execution.Values()
	execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json", Args: map[string]any{"status": http.StatusOK}})
	return "", nil
}

type schemaOnErrorStepExecutor struct {
	runs       int
	onErrorRun bool
}

func (s *schemaOnErrorStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	s.runs++
	return "", nil
}

func (s *schemaOnErrorStepExecutor) ExecuteOnErrorHandler(execution *Execution, body string, fe *FlowError) error {
	s.onErrorRun = true
	execution.State().SetResponse(&ResponseDescriptor{
		HandlerName: "http.json",
		Args:        map[string]any{"status": http.StatusTeapot, "body": map[string]any{"code": fe.Code}},
	})
	return nil
}

func (s *schemaOnErrorStepExecutor) ExecuteCompensation(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error {
	return nil
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

func (s *testValueStore) Snapshot() map[string]any {
	out := make(map[string]any, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
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
	stepExecutor := noopStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))

	registerFlowRoute(flow, container, executor, nil, newTestValueStore, router)

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
	stepExecutor := noopStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))

	registerFlowRoute(flow, container, executor, nil, newTestValueStore, router)

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
	stepExecutor := responseDescriptorStepExecutor{
		descriptor: &ResponseDescriptor{HandlerName: "missing.handler"},
	}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))

	registerFlowRoute(flow, container, executor, nil, newTestValueStore, router)

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
	if !strings.Contains(flowSpan.Status().Description, "unsupported HTTP response subtype") {
		t.Fatalf("expected root span error description, got %q", flowSpan.Status().Description)
	}
	if value, ok := spanAttribute(flowSpan.Attributes(), attribute.Key("http.status_code")); !ok || value.AsInt64() != http.StatusInternalServerError {
		t.Fatalf("expected root span http.status_code=500, got %v (present=%v)", value, ok)
	}
}

func TestHandleRequest_ValidatesAndNormalizesInputs(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	container := NewContainer(NewLogger(NewObservabilityLoggerWithWriter(&bytes.Buffer{}, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})))

	router := gin.New()
	flow := &Flow{
		ID: "orders",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "POST",
				"path":   "/orders/:id",
				"body": map[string]any{
					"type": "json",
				},
			},
			Input: mustParseHTTPBinding(t, map[string]any{
				"pathVariables": map[string]any{"id": map[string]any{"type": "integer", "required": true}},
				"queryParameters": map[string]any{
					"limit": map[string]any{"type": "integer", "default": "50", "minimum": "1"},
				},
				"headers": map[string]any{"X-Enabled": map[string]any{"type": "boolean", "required": true}},
				"body": map[string]any{
					"type": "json",
					"schema": map[string]any{
						"customer_email": map[string]any{"type": "string", "required": true, "format": "email"},
					},
				},
			}),
		},
		Steps: []Step{{ID: "capture"}},
	}
	stepExecutor := &captureInputStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))
	registerFlowRoute(flow, container, executor, nil, func() ValueStore { return NewValueStore() }, router)

	req := httptest.NewRequest(http.MethodPost, "/orders/42", strings.NewReader(`{"customer_email":"alice@example.com","extra":"kept"}`))
	req.Header.Set("X-Enabled", "true")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if stepExecutor.runs != 1 {
		t.Fatalf("step runs = %d, want 1", stepExecutor.runs)
	}
	request := stepExecutor.input["request"].(map[string]any)
	pathVariables := request["pathVariables"].(map[string]any)
	if got := pathVariables["id"]; got != int64(42) {
		t.Fatalf("path id = %#v (%T), want int64(42)", got, got)
	}
	queryParameters := request["queryParameters"].(map[string]any)
	if got := queryParameters["limit"]; got != int64(50) {
		t.Fatalf("query limit = %#v (%T), want int64(50)", got, got)
	}
	headers := request["headers"].(map[string]any)
	if got := headers["X-Enabled"]; got != true {
		t.Fatalf("header X-Enabled = %#v (%T), want true", got, got)
	}
	body := request["body"].(map[string]any)
	if got := body["extra"]; got != "kept" {
		t.Fatalf("body extra = %v, want kept", got)
	}
}

func TestHandleRequest_SchemaViolationReturnsProblemAndSkipsSteps(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	container := NewContainer(NewLogger(NewObservabilityLoggerWithWriter(&bytes.Buffer{}, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})))

	router := gin.New()
	flow := &Flow{
		ID: "orders",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "POST",
				"path":   "/orders",
				"body":   map[string]any{"type": "json"},
			},
			Input: mustParseHTTPBinding(t, map[string]any{
				"body": map[string]any{
					"type": "json",
					"schema": map[string]any{
						"customer_email": map[string]any{"type": "string", "required": true},
					},
				},
			}),
		},
		Steps: []Step{{ID: "capture"}},
	}
	stepExecutor := &captureInputStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))
	registerFlowRoute(flow, container, executor, nil, func() ValueStore { return NewValueStore() }, router)

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if stepExecutor.runs != 0 {
		t.Fatalf("step runs = %d, want 0", stepExecutor.runs)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("content-type = %q, want problem+json", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "body.customer_email") {
		t.Fatalf("problem response missing field error: %s", rec.Body.String())
	}
}

func TestHandleRequest_SchemaViolationCanBeHandledByOnError(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	container := NewContainer(NewLogger(NewObservabilityLoggerWithWriter(&bytes.Buffer{}, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})))

	router := gin.New()
	flow := &Flow{
		ID: "orders",
		Entrypoint: Entrypoint{
			Type: "http",
			Config: map[string]any{
				"method": "POST",
				"path":   "/orders",
				"body":   map[string]any{"type": "json"},
			},
			Input: mustParseHTTPBinding(t, map[string]any{
				"body": map[string]any{
					"type": "json",
					"schema": map[string]any{
						"customer_email": map[string]any{"type": "string", "required": true},
					},
				},
			}),
		},
		OnErrorBody: `response.json({status: 418})`,
		Steps:       []Step{{ID: "capture"}},
	}
	stepExecutor := &schemaOnErrorStepExecutor{}
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))
	registerFlowRoute(flow, container, executor, nil, func() ValueStore { return NewValueStore() }, router)

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d: %s", rec.Code, rec.Body.String())
	}
	if stepExecutor.runs != 0 {
		t.Fatalf("step runs = %d, want 0", stepExecutor.runs)
	}
	if !stepExecutor.onErrorRun {
		t.Fatal("expected on_error handler to run")
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
