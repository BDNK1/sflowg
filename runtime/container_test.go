package runtime

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type failingInitializerPlugin struct{}

func (p *failingInitializerPlugin) Initialize(_ Logger) error {
	return errors.New("boom")
}

func TestContainerInitialize_ReturnsNamedPluginError(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("payments", &failingInitializerPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	err := container.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to return an error")
	}
	if !strings.Contains(err.Error(), `plugin "payments" initialization failed`) {
		t.Fatalf("expected named plugin error, got %v", err)
	}
}

type greetingInput struct {
	Name string `json:"name" validate:"required"`
}

type greetingOutput struct {
	Message string `json:"message"`
}

type greetingPlugin struct{}

func (p *greetingPlugin) Greet(exec *Execution, input greetingInput) (greetingOutput, error) {
	return greetingOutput{Message: "hello " + input.Name}, nil
}

func (p *greetingPlugin) Json(c *gin.Context, exec *Execution, args map[string]any) error {
	c.Header("X-Plugin-Message", args["message"].(string))
	return nil
}

func TestContainerRegisterPlugin_RegistersTypedTask(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("greeting", &greetingPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	task := container.GetTask("greeting.greet")
	if task == nil {
		t.Fatal("expected typed task to be registered")
	}

	exec := &Execution{
		Container: container,
		ctx:       context.Background(),
	}

	result, err := task.Execute(exec, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["message"] != "hello world" {
		t.Fatalf("expected message to round-trip through typed task wrapper, got %#v", result["message"])
	}
}

func TestContainerRegisterPlugin_TypedTaskValidationFailure(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("greeting", &greetingPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	task := container.GetTask("greeting.greet")
	if task == nil {
		t.Fatal("expected typed task to be registered")
	}

	exec := &Execution{
		Container: container,
		ctx:       context.Background(),
	}

	_, err := task.Execute(exec, map[string]any{})
	if err == nil {
		t.Fatal("expected validation failure for missing required input")
	}
	if !strings.Contains(err.Error(), "validation failed for task greet") {
		t.Fatalf("expected typed wrapper validation error, got %v", err)
	}
}

func TestContainerRegisterPlugin_RegistersResponseHandler(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("greeting", &greetingPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	handler, ok := container.ResponseHandlers.Get("greeting.json")
	if !ok {
		t.Fatal("expected response handler to be registered")
	}

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)

	exec := &Execution{
		Container: container,
		ctx:       context.Background(),
	}

	if err := handler.Handle(ginCtx, exec, map[string]any{"message": "ok"}); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if got := recorder.Header().Get("X-Plugin-Message"); got != "ok" {
		t.Fatalf("expected plugin response handler to run, got header %q", got)
	}
}
