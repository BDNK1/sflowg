package runtime

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type noopEvaluator struct{}

func (noopEvaluator) Eval(execution *Execution, expression string) (any, error) {
	return nil, nil
}

type noopStepExecutor struct{}

func (noopStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
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
