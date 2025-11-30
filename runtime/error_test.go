package runtime

import (
	"errors"
	"testing"
)

// Test TaskError creation
func TestNewTaskError(t *testing.T) {
	baseErr := errors.New("something went wrong")
	taskErr := NewTaskError(baseErr)

	if taskErr.Err != baseErr {
		t.Errorf("Expected underlying error to be %v, got %v", baseErr, taskErr.Err)
	}

	if taskErr.Metadata == nil {
		t.Error("Expected metadata map to be initialized")
	}

	if len(taskErr.Metadata) != 0 {
		t.Errorf("Expected empty metadata map, got %d entries", len(taskErr.Metadata))
	}
}

// Test TaskError.Error() interface
func TestTaskError_Error(t *testing.T) {
	baseErr := errors.New("base error message")
	taskErr := NewTaskError(baseErr)

	if taskErr.Error() != "base error message" {
		t.Errorf("Expected error message 'base error message', got '%s'", taskErr.Error())
	}

	// Test with nil error (warnings-only)
	warningErr := &TaskError{
		Err:      nil,
		Metadata: make(map[string]any),
	}

	if warningErr.Error() != "task completed with metadata" {
		t.Errorf("Expected warning message, got '%s'", warningErr.Error())
	}
}

// Test TaskError.Unwrap() for errors.As compatibility
func TestTaskError_Unwrap(t *testing.T) {
	baseErr := errors.New("base error")
	taskErr := NewTaskError(baseErr)

	unwrapped := taskErr.Unwrap()
	if unwrapped != baseErr {
		t.Errorf("Expected unwrapped error to be %v, got %v", baseErr, unwrapped)
	}

	// Test with errors.As
	var target *TaskError
	if !errors.As(taskErr, &target) {
		t.Error("errors.As should find TaskError")
	}

	if target.Err != baseErr {
		t.Errorf("errors.As target error mismatch: expected %v, got %v", baseErr, target.Err)
	}
}

// Test WithMetadata
func TestTaskError_WithMetadata(t *testing.T) {
	taskErr := NewTaskError(errors.New("test"))

	result := taskErr.WithMetadata("key1", "value1")

	// Check chainable
	if result != taskErr {
		t.Error("WithMetadata should return same instance for chaining")
	}

	if taskErr.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1='value1', got '%v'", taskErr.Metadata["key1"])
	}

	// Add multiple keys
	taskErr.
		WithMetadata("key2", 42).
		WithMetadata("key3", true)

	if taskErr.Metadata["key2"] != 42 {
		t.Errorf("Expected metadata key2=42, got %v", taskErr.Metadata["key2"])
	}

	if taskErr.Metadata["key3"] != true {
		t.Errorf("Expected metadata key3=true, got %v", taskErr.Metadata["key3"])
	}
}

// Test WithMetadataMap
func TestTaskError_WithMetadataMap(t *testing.T) {
	taskErr := NewTaskError(errors.New("test"))

	metadata := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	result := taskErr.WithMetadataMap(metadata)

	// Check chainable
	if result != taskErr {
		t.Error("WithMetadataMap should return same instance for chaining")
	}

	// Verify all entries were added
	for k, v := range metadata {
		if taskErr.Metadata[k] != v {
			t.Errorf("Expected metadata %s=%v, got %v", k, v, taskErr.Metadata[k])
		}
	}
}

// Test WithRetryHint
func TestTaskError_WithRetryHint(t *testing.T) {
	taskErr := NewTaskError(errors.New("transient error"))

	result := taskErr.WithRetryHint(true, "5s")

	// Check chainable
	if result != taskErr {
		t.Error("WithRetryHint should return same instance for chaining")
	}

	if taskErr.Metadata["retryable"] != true {
		t.Errorf("Expected retryable=true, got %v", taskErr.Metadata["retryable"])
	}

	if taskErr.Metadata["retry_after"] != "5s" {
		t.Errorf("Expected retry_after='5s', got '%v'", taskErr.Metadata["retry_after"])
	}

	// Test without retry_after
	taskErr2 := NewTaskError(errors.New("test")).WithRetryHint(false, "")

	if taskErr2.Metadata["retryable"] != false {
		t.Errorf("Expected retryable=false, got %v", taskErr2.Metadata["retryable"])
	}

	if _, exists := taskErr2.Metadata["retry_after"]; exists {
		t.Error("retry_after should not be set when empty string provided")
	}
}

// Test WithType
func TestTaskError_WithType(t *testing.T) {
	taskErr := NewTaskError(errors.New("test"))

	result := taskErr.WithType("transient")

	// Check chainable
	if result != taskErr {
		t.Error("WithType should return same instance for chaining")
	}

	if taskErr.Metadata["type"] != "transient" {
		t.Errorf("Expected type='transient', got '%v'", taskErr.Metadata["type"])
	}
}

// Test IsRetryable
func TestTaskError_IsRetryable(t *testing.T) {
	// Test retryable error
	taskErr1 := NewTaskError(errors.New("test")).WithRetryHint(true, "")

	if !taskErr1.IsRetryable() {
		t.Error("Expected IsRetryable() to return true")
	}

	// Test non-retryable error
	taskErr2 := NewTaskError(errors.New("test")).WithRetryHint(false, "")

	if taskErr2.IsRetryable() {
		t.Error("Expected IsRetryable() to return false")
	}

	// Test error without retry metadata
	taskErr3 := NewTaskError(errors.New("test"))

	if taskErr3.IsRetryable() {
		t.Error("Expected IsRetryable() to return false when metadata not set")
	}

	// Test with invalid type
	taskErr4 := NewTaskError(errors.New("test"))
	taskErr4.Metadata["retryable"] = "yes" // Invalid type (string instead of bool)

	if taskErr4.IsRetryable() {
		t.Error("Expected IsRetryable() to return false for invalid metadata type")
	}
}

// Test GetRetryAfter
func TestTaskError_GetRetryAfter(t *testing.T) {
	// Test with retry_after set
	taskErr1 := NewTaskError(errors.New("test")).WithRetryHint(true, "10s")

	if taskErr1.GetRetryAfter() != "10s" {
		t.Errorf("Expected GetRetryAfter() to return '10s', got '%s'", taskErr1.GetRetryAfter())
	}

	// Test without retry_after
	taskErr2 := NewTaskError(errors.New("test"))

	if taskErr2.GetRetryAfter() != "" {
		t.Errorf("Expected GetRetryAfter() to return empty string, got '%s'", taskErr2.GetRetryAfter())
	}

	// Test with invalid type
	taskErr3 := NewTaskError(errors.New("test"))
	taskErr3.Metadata["retry_after"] = 123 // Invalid type (int instead of string)

	if taskErr3.GetRetryAfter() != "" {
		t.Errorf("Expected GetRetryAfter() to return empty string for invalid type, got '%s'", taskErr3.GetRetryAfter())
	}
}

// Test GetType
func TestTaskError_GetType(t *testing.T) {
	// Test with type set
	taskErr1 := NewTaskError(errors.New("test")).WithType("permanent")

	if taskErr1.GetType() != "permanent" {
		t.Errorf("Expected GetType() to return 'permanent', got '%s'", taskErr1.GetType())
	}

	// Test without type
	taskErr2 := NewTaskError(errors.New("test"))

	if taskErr2.GetType() != "" {
		t.Errorf("Expected GetType() to return empty string, got '%s'", taskErr2.GetType())
	}

	// Test with invalid type
	taskErr3 := NewTaskError(errors.New("test"))
	taskErr3.Metadata["type"] = true // Invalid type (bool instead of string)

	if taskErr3.GetType() != "" {
		t.Errorf("Expected GetType() to return empty string for invalid type, got '%s'", taskErr3.GetType())
	}
}

// Test method chaining
func TestTaskError_MethodChaining(t *testing.T) {
	taskErr := NewTaskError(errors.New("test error")).
		WithType("transient").
		WithRetryHint(true, "5s").
		WithMetadata("attempt", 3).
		WithMetadata("duration_ms", 150)

	// Verify all metadata was set
	if taskErr.GetType() != "transient" {
		t.Error("Expected type to be 'transient'")
	}

	if !taskErr.IsRetryable() {
		t.Error("Expected error to be retryable")
	}

	if taskErr.GetRetryAfter() != "5s" {
		t.Error("Expected retry_after to be '5s'")
	}

	if taskErr.Metadata["attempt"] != 3 {
		t.Errorf("Expected attempt=3, got %v", taskErr.Metadata["attempt"])
	}

	if taskErr.Metadata["duration_ms"] != 150 {
		t.Errorf("Expected duration_ms=150, got %v", taskErr.Metadata["duration_ms"])
	}
}

// Test nil error (warnings-only scenario)
func TestTaskError_NilError(t *testing.T) {
	taskErr := &TaskError{
		Err: nil,
		Metadata: map[string]any{
			"warnings": []string{"deprecated API used", "slow query detected"},
		},
	}

	if taskErr.Error() != "task completed with metadata" {
		t.Errorf("Expected warning message, got '%s'", taskErr.Error())
	}

	if taskErr.Unwrap() != nil {
		t.Error("Expected Unwrap() to return nil for warnings-only error")
	}

	if warnings, ok := taskErr.Metadata["warnings"].([]string); ok {
		if len(warnings) != 2 {
			t.Errorf("Expected 2 warnings, got %d", len(warnings))
		}
	} else {
		t.Error("Expected warnings in metadata")
	}
}
