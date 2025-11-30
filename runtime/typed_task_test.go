package runtime

import (
	"fmt"
	"testing"
)

// Test plugin with typed methods
type TestTypedPlugin struct{}

// Typed input/output structs
type GreetInput struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"gte=0,lte=150"`
}

type GreetOutput struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// Typed task method
func (p *TestTypedPlugin) Greet(exec *Execution, input GreetInput) (GreetOutput, error) {
	if input.Name == "" {
		return GreetOutput{}, fmt.Errorf("name is required")
	}

	message := fmt.Sprintf("Hello, %s! You are %d years old.", input.Name, input.Age)
	return GreetOutput{
		Message: message,
		Success: true,
	}, nil
}

// Typed task that returns an error
func (p *TestTypedPlugin) FailTask(exec *Execution, input GreetInput) (GreetOutput, error) {
	return GreetOutput{}, fmt.Errorf("intentional failure")
}

// Map-based task for backward compatibility test
func (p *TestTypedPlugin) MapBased(exec *Execution, args map[string]any) (map[string]any, error) {
	name := args["name"].(string)
	return map[string]any{
		"result": "map-based: " + name,
	}, nil
}

// Test plugin with mixed signatures
type MixedPlugin struct{}

type CalculateInput struct {
	A int `json:"a" validate:"required"`
	B int `json:"b" validate:"required"`
}

type CalculateOutput struct {
	Sum     int `json:"sum"`
	Product int `json:"product"`
}

// Typed method
func (p *MixedPlugin) Calculate(exec *Execution, input CalculateInput) (CalculateOutput, error) {
	return CalculateOutput{
		Sum:     input.A + input.B,
		Product: input.A * input.B,
	}, nil
}

// Map-based method
func (p *MixedPlugin) Echo(exec *Execution, args map[string]any) (map[string]any, error) {
	return map[string]any{
		"echo": args["message"],
	}, nil
}

// Test typed task registration and execution
func TestTypedTask_Registration(t *testing.T) {
	container := NewContainer()

	plugin := &TestTypedPlugin{}
	err := container.RegisterPlugin("test", plugin)

	if err != nil {
		t.Fatalf("Failed to register plugin: %v", err)
	}

	// Verify task was registered
	task := container.GetTask("test.greet")
	if task == nil {
		t.Fatal("Expected task 'test.greet' to be registered")
	}

	// Verify it's a typed wrapper
	if _, ok := task.(*typedTaskWrapper); !ok {
		t.Errorf("Expected typedTaskWrapper, got %T", task)
	}
}

// Test typed task execution
func TestTypedTask_Execution(t *testing.T) {
	container := NewContainer()
	plugin := &TestTypedPlugin{}
	container.RegisterPlugin("test", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	// Get task
	task := container.GetTask("test.greet")

	// Execute with map input
	input := map[string]any{
		"name": "Alice",
		"age":  30,
	}

	result, err := task.Execute(exec, input)

	if err != nil {
		t.Fatalf("Task execution failed: %v", err)
	}

	// Verify output
	if result["message"] == nil {
		t.Fatal("Expected 'message' field in output")
	}

	expectedMessage := "Hello, Alice! You are 30 years old."
	if result["message"] != expectedMessage {
		t.Errorf("Expected message '%s', got '%v'", expectedMessage, result["message"])
	}

	if result["success"] != true {
		t.Errorf("Expected success to be true, got %v", result["success"])
	}
}

// Test typed task validation
func TestTypedTask_Validation(t *testing.T) {
	container := NewContainer()
	plugin := &TestTypedPlugin{}
	container.RegisterPlugin("test", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	task := container.GetTask("test.greet")

	// Test with missing required field
	t.Run("MissingRequiredField", func(t *testing.T) {
		input := map[string]any{
			"age": 30,
			// "name" is missing but required
		}

		_, err := task.Execute(exec, input)

		if err == nil {
			t.Error("Expected validation error for missing 'name', got nil")
		}
	})

	// Test with invalid age
	t.Run("InvalidAge", func(t *testing.T) {
		input := map[string]any{
			"name": "Bob",
			"age":  200, // Exceeds max of 150
		}

		_, err := task.Execute(exec, input)

		if err == nil {
			t.Error("Expected validation error for age > 150, got nil")
		}
	})

	// Test with valid input
	t.Run("ValidInput", func(t *testing.T) {
		input := map[string]any{
			"name": "Charlie",
			"age":  25,
		}

		result, err := task.Execute(exec, input)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
	})
}

// Test typed task error handling
func TestTypedTask_ErrorHandling(t *testing.T) {
	container := NewContainer()
	plugin := &TestTypedPlugin{}
	container.RegisterPlugin("test", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	task := container.GetTask("test.failTask")

	input := map[string]any{
		"name": "Test",
		"age":  30,
	}

	_, err := task.Execute(exec, input)

	if err == nil {
		t.Fatal("Expected error from FailTask, got nil")
	}

	expectedError := "intentional failure"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%v'", expectedError, err.Error())
	}
}

// Test mixed signatures in same plugin
func TestMixedSignatures(t *testing.T) {
	container := NewContainer()
	plugin := &MixedPlugin{}
	container.RegisterPlugin("mixed", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	// Test typed method
	t.Run("TypedMethod", func(t *testing.T) {
		task := container.GetTask("mixed.calculate")
		if task == nil {
			t.Fatal("Expected task 'mixed.calculate' to be registered")
		}

		// Verify it's a typed wrapper
		if _, ok := task.(*typedTaskWrapper); !ok {
			t.Errorf("Expected typedTaskWrapper for Calculate, got %T", task)
		}

		input := map[string]any{
			"a": 5,
			"b": 3,
		}

		result, err := task.Execute(exec, input)
		if err != nil {
			t.Fatalf("Calculate execution failed: %v", err)
		}

		// JSON unmarshaling converts to float64
		if result["sum"] != float64(8) {
			t.Errorf("Expected sum 8, got %v", result["sum"])
		}

		if result["product"] != float64(15) {
			t.Errorf("Expected product 15, got %v", result["product"])
		}
	})

	// Test map-based method
	t.Run("MapBasedMethod", func(t *testing.T) {
		task := container.GetTask("mixed.echo")
		if task == nil {
			t.Fatal("Expected task 'mixed.echo' to be registered")
		}

		// Verify it's NOT a typed wrapper
		if _, ok := task.(*typedTaskWrapper); ok {
			t.Error("Expected pluginTaskWrapper for Echo, got typedTaskWrapper")
		}

		input := map[string]any{
			"message": "Hello World",
		}

		result, err := task.Execute(exec, input)
		if err != nil {
			t.Fatalf("Echo execution failed: %v", err)
		}

		if result["echo"] != "Hello World" {
			t.Errorf("Expected echo 'Hello World', got '%v'", result["echo"])
		}
	})
}

// Test backward compatibility with existing map-based tasks
func TestBackwardCompatibility_MapBasedTasks(t *testing.T) {
	container := NewContainer()
	plugin := &TestTypedPlugin{}
	container.RegisterPlugin("test", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	// Get map-based task
	task := container.GetTask("test.mapBased")
	if task == nil {
		t.Fatal("Expected task 'test.mapbased' to be registered")
	}

	// Verify it's a pluginTaskWrapper (not typed)
	if _, ok := task.(*pluginTaskWrapper); !ok {
		t.Errorf("Expected pluginTaskWrapper, got %T", task)
	}

	// Execute map-based task
	input := map[string]any{
		"name": "Test User",
	}

	result, err := task.Execute(exec, input)
	if err != nil {
		t.Fatalf("Map-based task execution failed: %v", err)
	}

	expected := "map-based: Test User"
	if result["result"] != expected {
		t.Errorf("Expected result '%s', got '%v'", expected, result["result"])
	}
}

// Test type conversion edge cases
func TestTypedTask_TypeConversion(t *testing.T) {
	container := NewContainer()
	plugin := &MixedPlugin{}
	container.RegisterPlugin("mixed", plugin)

	exec := &Execution{
		ID:        "test-exec",
		Values:    make(map[string]any),
		Container: container,
	}

	task := container.GetTask("mixed.calculate")

	// Test with string numbers (should be coerced)
	t.Run("StringToIntCoercion", func(t *testing.T) {
		input := map[string]any{
			"a": "10",
			"b": "5",
		}

		result, err := task.Execute(exec, input)
		if err != nil {
			t.Fatalf("Execution failed: %v", err)
		}

		if result["sum"] != float64(15) {
			t.Errorf("Expected sum 15, got %v", result["sum"])
		}
	})

	// Test with float inputs (should work with weak typing)
	t.Run("FloatToIntCoercion", func(t *testing.T) {
		input := map[string]any{
			"a": 7.9, // Should be coerced to 7
			"b": 3.1, // Should be coerced to 3
		}

		result, err := task.Execute(exec, input)
		if err != nil {
			t.Fatalf("Execution failed: %v", err)
		}

		// Verify coercion happened
		if result["sum"] == nil {
			t.Error("Expected sum to be present")
		}
	})
}
