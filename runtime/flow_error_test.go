package runtime

import "testing"

func TestFlowErrorToMap_NamespacesMeta(t *testing.T) {
	fe := &FlowError{
		Type:    ErrorTypePermanent,
		Code:    "SCHEMA_VIOLATION",
		Message: "Request validation failed",
		Step:    "validate",
		Retries: 2,
		Meta: map[string]any{
			"code":   "SHADOWED",
			"fields": []any{"field-error"},
		},
	}

	got := fe.ToMap()
	if got["code"] != "SCHEMA_VIOLATION" {
		t.Fatalf("code = %v, want SCHEMA_VIOLATION", got["code"])
	}
	if got["message"] != "Request validation failed" {
		t.Fatalf("message = %v", got["message"])
	}
	if got["step"] != "validate" {
		t.Fatalf("step = %v", got["step"])
	}
	if got["retries"] != 2 {
		t.Fatalf("retries = %v", got["retries"])
	}
	if _, ok := got["fields"]; ok {
		t.Fatalf("metadata leaked to top level: %#v", got)
	}

	meta, ok := got["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want map[string]any", got["meta"])
	}
	if meta["code"] != "SHADOWED" {
		t.Fatalf("meta.code = %v, want SHADOWED", meta["code"])
	}
	if fields, ok := meta["fields"].([]any); !ok || fields[0] != "field-error" {
		t.Fatalf("meta.fields = %#v", meta["fields"])
	}
}

func TestFlowErrorToMap_OmitsEmptyMeta(t *testing.T) {
	got := (&FlowError{Type: ErrorTypePermanent, Code: "FAIL"}).ToMap()
	if _, ok := got["meta"]; ok {
		t.Fatalf("unexpected meta key: %#v", got)
	}
}
