package runtime

import (
	"testing"
	"time"
)

// Test types for conversion
type SimpleStruct struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email"`
}

type StructWithDuration struct {
	Timeout  time.Duration `json:"timeout"`
	Interval time.Duration `json:"interval"`
}

type NestedStruct struct {
	Title   string       `json:"title"`
	Details SimpleStruct `json:"details"`
	Count   int          `json:"count"`
}

type StructWithTags struct {
	PublicName  string `json:"public_name"`
	PrivateName string `json:"-"` // Should be ignored
	OmitEmpty   string `json:"omit_empty,omitempty"`
}

// Test mapToStruct with basic types
func TestMapToStruct_BasicTypes(t *testing.T) {
	input := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john@example.com",
	}

	var result SimpleStruct
	err := mapToStruct(input, &result)

	if err != nil {
		t.Fatalf("mapToStruct failed: %v", err)
	}

	if result.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", result.Name)
	}

	if result.Age != 30 {
		t.Errorf("Expected age 30, got %d", result.Age)
	}

	if result.Email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got '%s'", result.Email)
	}
}

// Test mapToStruct with type coercion
func TestMapToStruct_TypeCoercion(t *testing.T) {
	// Test with string that should be converted to int
	input := map[string]any{
		"name":  "Jane",
		"age":   "25", // String instead of int
		"email": "jane@example.com",
	}

	var result SimpleStruct
	err := mapToStruct(input, &result)

	if err != nil {
		t.Fatalf("mapToStruct failed: %v", err)
	}

	if result.Age != 25 {
		t.Errorf("Expected age 25, got %d", result.Age)
	}
}

// Test mapToStruct with time.Duration
func TestMapToStruct_Duration(t *testing.T) {
	input := map[string]any{
		"timeout":  "30s",
		"interval": "5m",
	}

	var result StructWithDuration
	err := mapToStruct(input, &result)

	if err != nil {
		t.Fatalf("mapToStruct failed: %v", err)
	}

	expectedTimeout := 30 * time.Second
	if result.Timeout != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, result.Timeout)
	}

	expectedInterval := 5 * time.Minute
	if result.Interval != expectedInterval {
		t.Errorf("Expected interval %v, got %v", expectedInterval, result.Interval)
	}
}

// Test mapToStruct with nested structs
func TestMapToStruct_NestedStruct(t *testing.T) {
	input := map[string]any{
		"title": "Test",
		"details": map[string]any{
			"name":  "Nested",
			"age":   40,
			"email": "nested@example.com",
		},
		"count": 5,
	}

	var result NestedStruct
	err := mapToStruct(input, &result)

	if err != nil {
		t.Fatalf("mapToStruct failed: %v", err)
	}

	if result.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", result.Title)
	}

	if result.Details.Name != "Nested" {
		t.Errorf("Expected nested name 'Nested', got '%s'", result.Details.Name)
	}

	if result.Count != 5 {
		t.Errorf("Expected count 5, got %d", result.Count)
	}
}

// Test structToMap with basic types
func TestStructToMap_BasicTypes(t *testing.T) {
	input := SimpleStruct{
		Name:  "Alice",
		Age:   28,
		Email: "alice@example.com",
	}

	result, err := structToMap(input)

	if err != nil {
		t.Fatalf("structToMap failed: %v", err)
	}

	if result["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got '%v'", result["name"])
	}

	// JSON unmarshaling converts numbers to float64
	if result["age"] != float64(28) {
		t.Errorf("Expected age 28, got %v", result["age"])
	}

	if result["email"] != "alice@example.com" {
		t.Errorf("Expected email 'alice@example.com', got '%v'", result["email"])
	}
}

// Test structToMap with nested structs
func TestStructToMap_NestedStruct(t *testing.T) {
	input := NestedStruct{
		Title: "Nested Test",
		Details: SimpleStruct{
			Name:  "Bob",
			Age:   35,
			Email: "bob@example.com",
		},
		Count: 10,
	}

	result, err := structToMap(input)

	if err != nil {
		t.Fatalf("structToMap failed: %v", err)
	}

	if result["title"] != "Nested Test" {
		t.Errorf("Expected title 'Nested Test', got '%v'", result["title"])
	}

	details, ok := result["details"].(map[string]any)
	if !ok {
		t.Fatalf("Expected details to be a map, got %T", result["details"])
	}

	if details["name"] != "Bob" {
		t.Errorf("Expected nested name 'Bob', got '%v'", details["name"])
	}

	if result["count"] != float64(10) {
		t.Errorf("Expected count 10, got %v", result["count"])
	}
}

// Test round-trip conversion (map → struct → map)
func TestRoundTripConversion(t *testing.T) {
	original := map[string]any{
		"name":  "Charlie",
		"age":   45,
		"email": "charlie@example.com",
	}

	// Convert map → struct
	var intermediate SimpleStruct
	err := mapToStruct(original, &intermediate)
	if err != nil {
		t.Fatalf("mapToStruct failed: %v", err)
	}

	// Convert struct → map
	result, err := structToMap(intermediate)
	if err != nil {
		t.Fatalf("structToMap failed: %v", err)
	}

	// Verify values match (accounting for type conversions)
	if result["name"] != original["name"] {
		t.Errorf("Name mismatch: expected '%v', got '%v'", original["name"], result["name"])
	}

	// JSON conversion turns int → float64
	if result["age"] != float64(original["age"].(int)) {
		t.Errorf("Age mismatch: expected %v, got %v", original["age"], result["age"])
	}

	if result["email"] != original["email"] {
		t.Errorf("Email mismatch: expected '%v', got '%v'", original["email"], result["email"])
	}
}

// Test mapToStruct with invalid input
func TestMapToStruct_InvalidInput(t *testing.T) {
	input := map[string]any{
		"name": "Test",
		"age":  "not a number", // Invalid type
	}

	var result SimpleStruct
	err := mapToStruct(input, &result)

	if err == nil {
		t.Error("Expected error for invalid type conversion, got nil")
	}
}

// Test structToMap respects json tags
func TestStructToMap_JSONTags(t *testing.T) {
	input := StructWithTags{
		PublicName:  "Public",
		PrivateName: "Private",
		OmitEmpty:   "",
	}

	result, err := structToMap(input)

	if err != nil {
		t.Fatalf("structToMap failed: %v", err)
	}

	// Should use json tag name
	if _, exists := result["public_name"]; !exists {
		t.Error("Expected 'public_name' field in result")
	}

	// Should ignore field with json:"-"
	if _, exists := result["private_name"]; exists {
		t.Error("Did not expect 'private_name' field in result")
	}

	if _, exists := result["PrivateName"]; exists {
		t.Error("Did not expect 'PrivateName' field in result")
	}
}
