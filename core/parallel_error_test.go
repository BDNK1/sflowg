package runtime

import "testing"

func TestFlowErrorToMapIncludesParallelFailuresWithoutRecursiveShape(t *testing.T) {
	child := &FlowError{
		Type:        ErrorTypePermanent,
		Code:        "FAIL",
		Message:     "boom",
		Step:        "branch_a",
		Retries:     2,
		Meta:        map[string]any{"k": "v"},
		AwaitedFrom: "async_a",
	}
	fe := NewParallelFailure("__parallel_1", []FlowFailure{{
		Branch: "branch_a",
		Step:   "branch_a",
		Error:  FlowErrorDetailFrom(child),
	}})
	if fe == nil {
		t.Fatal("NewParallelFailure returned nil")
	}
	m := fe.ToMap()
	failures, ok := m["failures"].([]map[string]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures = %#v", m["failures"])
	}
	detail, ok := failures[0]["error"].(map[string]any)
	if !ok {
		t.Fatalf("failure error = %#v", failures[0]["error"])
	}
	if _, recursive := detail["failures"]; recursive {
		t.Fatalf("child error detail should not contain recursive failures: %#v", detail)
	}
	if detail["step"] != "branch_a" || detail["awaited_from"] != "async_a" || detail["retries"] != 2 {
		t.Fatalf("detail did not preserve fields: %#v", detail)
	}
}

func TestFlowErrorToMapIncludesIteration(t *testing.T) {
	iteration := 0
	fe := NewGroupedFailure("__foreach_1", "multiple foreach iterations failed", []FlowFailure{{
		Iteration: &iteration,
		Step:      "charge",
		Error: FlowErrorDetail{
			Type:    ErrorTypePermanent,
			Code:    "FAIL",
			Message: "boom",
			Step:    "charge",
		},
	}})

	failures := fe.ToMap()["failures"].([]map[string]any)
	if failures[0]["iteration"] != 0 {
		t.Fatalf("iteration = %#v, want 0", failures[0]["iteration"])
	}
}

func TestStampForeachErrorClonesMeta(t *testing.T) {
	original := &FlowError{Meta: map[string]any{"existing": "value"}}
	stamped := stampForeachError(original, "__foreach_1", 3)

	if stamped == original {
		t.Fatal("stampForeachError returned original pointer")
	}
	if original.Meta["foreach"] != nil || original.Meta["iteration"] != nil {
		t.Fatalf("original meta mutated: %#v", original.Meta)
	}
	if stamped.Meta["foreach"] != "__foreach_1" || stamped.Meta["iteration"] != 3 {
		t.Fatalf("stamped meta = %#v", stamped.Meta)
	}
}
