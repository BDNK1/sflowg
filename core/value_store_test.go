package runtime

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

func TestValueStore_Snapshot_ReturnsNestedMap(t *testing.T) {
	s := NewValueStore()

	s.Set("step1.result", "val1")
	s.Set("step2.result", "val2")

	snapshot := s.Snapshot()
	step1, ok := snapshot["step1"].(map[string]any)
	if !ok {
		t.Fatal("step1 is not a map")
	}
	if step1["result"] != "val1" {
		t.Errorf("step1.result = %v, want val1", step1["result"])
	}

	step2, ok := snapshot["step2"].(map[string]any)
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

func TestValueStore_Snapshot_DeepCopiesNestedMaps(t *testing.T) {
	s := NewValueStore()

	s.SetNested("response", map[string]any{
		"body": map[string]any{
			"id": "xyz",
		},
	})

	snapshot := s.Snapshot()
	response := snapshot["response"].(map[string]any)
	body := response["body"].(map[string]any)
	body["id"] = "mutated"

	v, ok := s.Get("response.body.id")
	if !ok || v != "xyz" {
		t.Fatalf("store was mutated through snapshot: got %v, %v; want xyz, true", v, ok)
	}
}

func TestValueStore_SetNested_DeepCopiesInputMaps(t *testing.T) {
	s := NewValueStore()
	input := map[string]any{
		"body": map[string]any{
			"id": "original",
		},
	}

	s.SetNested("response", input)
	input["body"].(map[string]any)["id"] = "mutated"

	v, ok := s.Get("response.body.id")
	if !ok || v != "original" {
		t.Fatalf("store was mutated through input map: got %v, %v; want original, true", v, ok)
	}
}

func TestCloneRunStateSnapshot_IsolatesNestedMaps(t *testing.T) {
	snapshot := map[string]any{
		"request": map[string]any{
			"body": map[string]any{"id": "original"},
		},
	}

	left := cloneRunStateSnapshot(snapshot)
	right := cloneRunStateSnapshot(snapshot)

	left.Store().Set("request.body.id", "left")

	gotRight, ok := right.Store().Get("request.body.id")
	if !ok || gotRight != "original" {
		t.Fatalf("right clone was mutated through left clone: got %v, %v; want original, true", gotRight, ok)
	}
	gotSnapshot := snapshot["request"].(map[string]any)["body"].(map[string]any)["id"]
	if gotSnapshot != "original" {
		t.Fatalf("source snapshot was mutated: got %v, want original", gotSnapshot)
	}
}

func TestValueStore_Snapshot_DeepCopiesSlices(t *testing.T) {
	s := NewValueStore()

	s.SetNested("response", map[string]any{
		"items": []any{
			map[string]any{"id": "a"},
		},
	})

	snapshot := s.Snapshot()
	response := snapshot["response"].(map[string]any)
	items := response["items"].([]any)
	items[0].(map[string]any)["id"] = "mutated"

	v, ok := s.Get("response.items")
	if !ok {
		t.Fatal("response.items not found")
	}
	gotItems, ok := v.([]any)
	if !ok {
		t.Fatalf("response.items is %T, want []any", v)
	}
	gotFirst, ok := gotItems[0].(map[string]any)
	if !ok {
		t.Fatalf("response.items[0] is %T, want map[string]any", gotItems[0])
	}
	if gotFirst["id"] != "a" {
		t.Fatalf("store slice element was mutated through snapshot: got %v, want a", gotFirst["id"])
	}
}

func TestValueStore_SnapshotKeys(t *testing.T) {
	s := NewValueStore()
	s.SetNested("response", map[string]any{"body": map[string]any{"id": "a"}})
	s.Set("unused", "x")

	snapshot := s.SnapshotKeys([]string{"response.body.id", "missing"})
	if len(snapshot) != 1 {
		t.Fatalf("SnapshotKeys len = %d, want 1: %#v", len(snapshot), snapshot)
	}
	if snapshot["response.body.id"] != "a" {
		t.Fatalf("response.body.id = %v, want a", snapshot["response.body.id"])
	}
}

func TestScopedValueStore_ReadsFrozenParentAndWritesLocalOnly(t *testing.T) {
	parent := NewValueStore()
	parent.Set("prior", "before")
	view := NewReadOnlyValueView(parent, []string{"prior"})
	parent.Set("prior", "after")

	scoped := NewScopedValueStore(view)
	if got, ok := scoped.Get("prior"); !ok || got != "before" {
		t.Fatalf("scoped prior = %v, %v; want before, true", got, ok)
	}

	scoped.Set("local", "value")
	if _, ok := parent.Get("local"); ok {
		t.Fatal("local write leaked to parent")
	}
	if got, ok := scoped.Get("local"); !ok || got != "value" {
		t.Fatalf("scoped local = %v, %v; want value, true", got, ok)
	}
}

func TestScopedValueStore_SnapshotMergesParentAndLocalNestedMaps(t *testing.T) {
	parent := NewValueStore()
	parent.SetNested("request", map[string]any{
		"body": map[string]any{"id": "parent-id"},
		"headers": map[string]any{
			"trace": "parent-trace",
			"keep":  "parent-keep",
		},
	})
	view := NewReadOnlyValueView(parent, nil)
	scoped := NewScopedValueStore(view)
	scoped.SetNested("request.headers", map[string]any{"trace": "local-trace"})

	snapshot := scoped.Snapshot()
	request := snapshot["request"].(map[string]any)
	body := request["body"].(map[string]any)
	headers := request["headers"].(map[string]any)

	if body["id"] != "parent-id" {
		t.Fatalf("parent body was not preserved: %#v", body)
	}
	if headers["trace"] != "local-trace" {
		t.Fatalf("local header did not take precedence: %#v", headers)
	}
	if headers["keep"] != "parent-keep" {
		t.Fatalf("parent sibling header was not preserved: %#v", headers)
	}
}

func TestReadOnlyValueView_PreservesDottedKeys(t *testing.T) {
	parent := NewValueStore()
	parent.SetNested("response", map[string]any{
		"body": map[string]any{
			"id": "abc",
		},
	})

	view := NewReadOnlyValueView(parent, []string{"response.body.id"})
	if got, ok := view.Get("response.body.id"); !ok || got != "abc" {
		t.Fatalf("view response.body.id = %v, %v; want abc, true", got, ok)
	}

	snapshot := view.SnapshotKeys([]string{"response.body.id"})
	if snapshot["response.body.id"] != "abc" {
		t.Fatalf("snapshot response.body.id = %v, want abc", snapshot["response.body.id"])
	}

	scoped := NewScopedValueStore(view)
	if got, ok := scoped.Get("response.body.id"); !ok || got != "abc" {
		t.Fatalf("scoped response.body.id = %v, %v; want abc, true", got, ok)
	}
}
