package configutil

import "testing"

func TestResolvePropertyMap_PassthroughPlainValues(t *testing.T) {
	input := map[string]any{
		"host": "localhost",
		"port": "8080",
	}
	result, err := ResolvePropertyMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["host"] != "localhost" {
		t.Errorf("expected host=localhost, got %v", result["host"])
	}
	if result["port"] != "8080" {
		t.Errorf("expected port=8080, got %v", result["port"])
	}
}

func TestResolvePropertyMap_ResolvesEnvVar(t *testing.T) {
	t.Setenv("TEST_RESOLVE_VAR", "resolved_value")
	input := map[string]any{
		"key": "${TEST_RESOLVE_VAR}",
	}
	result, err := ResolvePropertyMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "resolved_value" {
		t.Errorf("expected resolved_value, got %v", result["key"])
	}
}

func TestResolvePropertyMap_DefaultValue(t *testing.T) {
	input := map[string]any{
		"key": "${UNSET_VAR_WITH_DEFAULT:fallback}",
	}
	result, err := ResolvePropertyMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["key"] != "fallback" {
		t.Errorf("expected fallback, got %v", result["key"])
	}
}

func TestResolvePropertyMap_ErrorOnMissingRequired(t *testing.T) {
	input := map[string]any{
		"key": "${MISSING_REQUIRED_VAR}",
	}
	_, err := ResolvePropertyMap(input)
	if err == nil {
		t.Fatal("expected error for missing required env var")
	}
}

func TestResolvePropertyMap_NonStringPassthrough(t *testing.T) {
	input := map[string]any{
		"count":   42,
		"enabled": true,
	}
	result, err := ResolvePropertyMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["count"] != 42 {
		t.Errorf("expected 42, got %v", result["count"])
	}
	if result["enabled"] != true {
		t.Errorf("expected true, got %v", result["enabled"])
	}
}

func TestResolvePropertyMap_EmptyMap(t *testing.T) {
	result, err := ResolvePropertyMap(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}
