package dsl

import (
	"testing"
)

func TestValueStore_SetAndGet_Simple(t *testing.T) {
	s := NewValueStore()

	s.Set("key", "value")
	v, ok := s.Get("key")
	if !ok || v != "value" {
		t.Errorf("Get(key) = %v, %v; want value, true", v, ok)
	}
}

func TestValueStore_SetAndGet_Nested(t *testing.T) {
	s := NewValueStore()

	s.Set("step.result.body.id", "abc123")

	// Should be accessible via full path
	v, ok := s.Get("step.result.body.id")
	if !ok || v != "abc123" {
		t.Errorf("Get(step.result.body.id) = %v, %v; want abc123, true", v, ok)
	}

	// Intermediate maps should exist
	v, ok = s.Get("step.result.body")
	if !ok {
		t.Fatal("step.result.body not found")
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("step.result.body is %T, want map[string]any", v)
	}
	if m["id"] != "abc123" {
		t.Errorf("step.result.body.id = %v, want abc123", m["id"])
	}

	// Top level should be a nested map
	v, ok = s.Get("step")
	if !ok {
		t.Fatal("step not found")
	}
	topMap, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("step is %T, want map[string]any", v)
	}
	resultMap, ok := topMap["result"].(map[string]any)
	if !ok {
		t.Fatal("step.result is not a map")
	}
	bodyMap, ok := resultMap["body"].(map[string]any)
	if !ok {
		t.Fatal("step.result.body is not a map")
	}
	if bodyMap["id"] != "abc123" {
		t.Errorf("nested access id = %v, want abc123", bodyMap["id"])
	}
}

func TestValueStore_SetNested_ExpandsMap(t *testing.T) {
	s := NewValueStore()

	s.SetNested("response", map[string]any{
		"status_code": int64(200),
		"body": map[string]any{
			"id":   "xyz",
			"name": "test",
		},
	})

	// Root level
	v, ok := s.Get("response")
	if !ok {
		t.Fatal("response not found")
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("response is %T, want map", v)
	}

	// Nested path
	v, ok = s.Get("response.body.id")
	if !ok || v != "xyz" {
		t.Errorf("response.body.id = %v, %v; want xyz, true", v, ok)
	}

	v, ok = s.Get("response.status_code")
	if !ok || v != int64(200) {
		t.Errorf("response.status_code = %v, %v; want 200, true", v, ok)
	}
}

func TestValueStore_Get_NotFound(t *testing.T) {
	s := NewValueStore()

	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent key")
	}

	s.Set("a.b", "value")
	_, ok = s.Get("a.b.c")
	if ok {
		t.Error("expected not found for deeper path than stored")
	}
}

func TestValueStore_All_ReturnsNestedMap(t *testing.T) {
	s := NewValueStore()

	s.Set("step1.result", "val1")
	s.Set("step2.result", "val2")

	all := s.All()
	step1, ok := all["step1"].(map[string]any)
	if !ok {
		t.Fatal("step1 is not a map")
	}
	if step1["result"] != "val1" {
		t.Errorf("step1.result = %v, want val1", step1["result"])
	}

	step2, ok := all["step2"].(map[string]any)
	if !ok {
		t.Fatal("step2 is not a map")
	}
	if step2["result"] != "val2" {
		t.Errorf("step2.result = %v, want val2", step2["result"])
	}
}

func TestValueStore_OverwriteValue(t *testing.T) {
	s := NewValueStore()

	s.Set("key", "first")
	s.Set("key", "second")

	v, ok := s.Get("key")
	if !ok || v != "second" {
		t.Errorf("Get(key) = %v, want second", v)
	}
}
