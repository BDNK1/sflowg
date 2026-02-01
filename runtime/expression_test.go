package runtime

import (
	"testing"
)

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "simple string",
			expr:     `base64_encode("hello")`,
			expected: "aGVsbG8=",
		},
		{
			name:     "empty string",
			expr:     `base64_encode("")`,
			expected: "",
		},
		{
			name:     "with special chars",
			expr:     `base64_encode("user:password")`,
			expected: "dXNlcjpwYXNzd29yZA==",
		},
		{
			name:     "stripe key format",
			expr:     `base64_encode("sk_test_123:")`,
			expected: "c2tfdGVzdF8xMjM6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, map[string]any{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBase64Decode(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "simple string",
			expr:     `base64_decode("aGVsbG8=")`,
			expected: "hello",
		},
		{
			name:     "empty string",
			expr:     `base64_decode("")`,
			expected: "",
		},
		{
			name:     "with special chars",
			expr:     `base64_decode("dXNlcjpwYXNzd29yZA==")`,
			expected: "user:password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, map[string]any{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBase64WithContext(t *testing.T) {
	ctx := map[string]any{
		"api_key": "sk_test_abc123",
	}

	result, err := Eval(`"Basic " + base64_encode(api_key + ":")`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Basic c2tfdGVzdF9hYmMxMjM6"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestAllowUndefinedVariables(t *testing.T) {
	ctx := map[string]any{
		"exists":   "hello",
		"is_nil":   nil,
	}

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		{"existing value", "exists", "hello"},
		{"nil value", "is_nil", nil},
		{"missing variable returns nil", "missing", nil},
		{"missing nested returns nil", "missing.nested.deep", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNullCoalescing(t *testing.T) {
	ctx := map[string]any{
		"has_value": "hello",
		"is_nil":    nil,
	}

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		{"value ?? default returns value", `has_value ?? "default"`, "hello"},
		{"nil ?? default returns default", `is_nil ?? "default"`, "default"},
		{"missing ?? default returns default", `missing ?? "default"`, "default"},
		{"chained coalescing", `missing ?? is_nil ?? "fallback"`, "fallback"},
		{"first non-nil wins", `missing ?? has_value ?? "fallback"`, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOptionalChaining(t *testing.T) {
	ctx := map[string]any{
		"user_name": "John",
		"user": map[string]any{
			"email": "john@example.com",
			"profile": map[string]any{
				"bio": "Hello",
			},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		{"existing path", "user_name", "John"},
		{"missing with ?.", "missing?.nested", nil},
		{"missing deep with ?.", "missing?.a?.b?.c", nil},
		{"existing nested with ?.", "user?.email", "john@example.com"},
		{"existing deep with ?.", "user?.profile?.bio", "Hello"},
		{"missing nested field with ?.", "user?.profile?.missing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

// NOTE: expr-lang limitation - optional chaining on explicitly nil values in context
// fails at compile time. This is acceptable because in real flows, missing steps
// return nil via AllowUndefinedVariables, and nested data uses maps not bare nil.

func TestDefinedFunction(t *testing.T) {
	ctx := map[string]any{
		"exists":        "hello",
		"is_nil":        nil,
		"step_result_id": 123,
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{"existing value is defined", `defined("exists")`, true},
		{"nil value is defined", `defined("is_nil")`, true},
		{"missing is not defined", `defined("missing")`, false},
		{"nested path with dots", `defined("step.result.id")`, true},
		{"missing nested path", `defined("step.result.missing")`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Eval(tt.expr, ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefinedWithConditional(t *testing.T) {
	// Simulate: step ran and returned nil vs step was skipped
	ctxStepRan := map[string]any{
		"step_result": nil,
	}
	ctxStepSkipped := map[string]any{
		// step.result not present
	}

	// Step ran but returned nil
	result, err := Eval(`defined("step.result") ? "ran" : "skipped"`, ctxStepRan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ran" {
		t.Errorf("got %v, want 'ran'", result)
	}

	// Step was skipped
	result, err = Eval(`defined("step.result") ? "ran" : "skipped"`, ctxStepSkipped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "skipped" {
		t.Errorf("got %v, want 'skipped'", result)
	}
}