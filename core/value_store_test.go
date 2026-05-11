package core

import (
	"fmt"
	"sync"
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

func TestValueStore_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	const (
		writers       = 16
		readers       = 16
		writesPerGoro = 200
		readsPerGoro  = 200
	)
	s := NewValueStore()
	s.SetNested("seed", map[string]any{"value": 0})

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		w := w
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerGoro; i++ {
				key := fmt.Sprintf("worker.%d.step.%d", w, i)
				s.SetNested(key, map[string]any{"i": i, "w": w})
				s.Set(fmt.Sprintf("flat.%d.%d", w, i), i)
			}
		}()
	}

	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerGoro; i++ {
				_, _ = s.Get("seed.value")
				_ = s.Snapshot()
			}
		}()
	}

	wg.Wait()
}

func TestValueStore_ConcurrentNestedWrites(t *testing.T) {
	t.Parallel()
	const goroutines = 32
	s := NewValueStore()

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			s.SetNested("shared", map[string]any{
				fmt.Sprintf("k%d", g): map[string]any{
					"nested": g,
					"list":   []any{g, g + 1},
				},
			})
		}()
	}
	wg.Wait()

	snap := s.Snapshot()
	shared, ok := snap["shared"].(map[string]any)
	if !ok {
		t.Fatalf("shared key not a map: %T", snap["shared"])
	}
	if len(shared) == 0 {
		t.Fatalf("shared map is empty after concurrent writes")
	}
}
