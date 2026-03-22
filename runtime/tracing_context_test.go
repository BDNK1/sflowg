package runtime

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type doneTouchStepExecutor struct{}

func (doneTouchStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	select {
	case <-ctx.Done():
	default:
	}

	select {
	case <-execution.Done():
	default:
	}

	return "", nil
}

type doneTouchPlugin struct{}

func (p *doneTouchPlugin) Touch(exec *Execution, args map[string]any) (map[string]any, error) {
	select {
	case <-exec.Done():
	default:
	}
	return map[string]any{"ok": true}, nil
}

func TestExecuteSteps_TracingContextDoesNotRecurse(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	provider := sdktrace.NewTracerProvider()
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()
	container.SetTracer(provider.Tracer("test"))

	flow := &Flow{
		ID:    "payments",
		Steps: []Step{{ID: "resolve_currency"}},
	}
	execution := NewExecution(flow, container, nil, newTestValueStore())
	execution = execution.WithContext(context.Background())

	executor := NewExecutor(noopEvaluator{}, doneTouchStepExecutor{})
	if err := executor.ExecuteSteps(execution); err != nil {
		t.Fatalf("expected ExecuteSteps to succeed, got %v", err)
	}
}

func TestPluginTask_TracingContextDoesNotRecurse(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	provider := sdktrace.NewTracerProvider()
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()
	container.SetTracer(provider.Tracer("test"))

	if err := container.RegisterPlugin("probe", &doneTouchPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	task := container.GetTask("probe.touch")
	if task == nil {
		t.Fatal("expected probe.touch task to be registered")
	}

	execution := NewExecution(&Flow{ID: "payments"}, container, nil, newTestValueStore())
	execution = execution.WithContext(context.Background())

	result, err := task.Execute(execution, map[string]any{})
	if err != nil {
		t.Fatalf("expected task execution to succeed, got %v", err)
	}
	if ok, exists := result["ok"]; !exists || ok != true {
		t.Fatalf("expected task result ok=true, got %v", result)
	}
}
