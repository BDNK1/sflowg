package schema

import "testing"

func TestValidate_AppliesDefaultBeforeRequired(t *testing.T) {
	s := &Schema{Type: TypeObject, Properties: map[string]*Schema{
		"currency": {Type: TypeString, Required: true, Default: "usd"},
	}}

	normalized, errs := Validate(map[string]any{}, s, "body")
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %#v", errs)
	}
	body := normalized.(map[string]any)
	if got := body["currency"]; got != "usd" {
		t.Fatalf("currency = %v, want usd", got)
	}
}

func TestValidate_PreservesUnknownFieldsAndNormalizesInteger(t *testing.T) {
	s := &Schema{Type: TypeObject, Properties: map[string]*Schema{
		"limit": {Type: TypeInteger, Minimum: floatPtr(1)},
	}}

	normalized, errs := Validate(map[string]any{"limit": "10", "extra": "kept"}, s, "queryParameters")
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %#v", errs)
	}
	obj := normalized.(map[string]any)
	if got := obj["limit"]; got != int64(10) {
		t.Fatalf("limit = %#v (%T), want int64(10)", got, got)
	}
	if got := obj["extra"]; got != "kept" {
		t.Fatalf("extra = %v, want kept", got)
	}
}

func TestValidate_ReportsRequiredField(t *testing.T) {
	s := &Schema{Type: TypeObject, Properties: map[string]*Schema{
		"customer_email": {Type: TypeString, Required: true},
	}}

	_, errs := Validate(map[string]any{}, s, "body")
	if len(errs) != 1 {
		t.Fatalf("validation errors len = %d, want 1: %#v", len(errs), errs)
	}
	if errs[0].Path != "body.customer_email" || errs[0].Constraint != "required" {
		t.Fatalf("unexpected error: %#v", errs[0])
	}
}

func TestParse_RejectsInvalidConstraintTypeCombinations(t *testing.T) {
	cases := []map[string]any{
		{"type": "string", "minimum": 1},
		{"type": "integer", "minLength": 1},
		{"type": "integer", "enum": []any{1, 2}},
		{"type": "number", "enum": []any{1.5}},
		{"type": "boolean", "enum": []any{true}},
		{"type": "object", "enum": []any{"x"}},
		{"type": "string", "items": map[string]any{"type": "string"}},
		{"type": "array"},
		{"type": "uuid"},
	}

	for _, tc := range cases {
		if _, err := Parse(tc); err == nil {
			t.Fatalf("expected Parse(%#v) to fail", tc)
		}
	}
}

func TestValidate_ArrayItemsAndFormats(t *testing.T) {
	s := &Schema{Type: TypeObject, Properties: map[string]*Schema{
		"ids":   {Type: TypeArray, Items: &Schema{Type: TypeString, Format: "uuid"}},
		"email": {Type: TypeString, Format: "email"},
	}}

	normalized, errs := Validate(map[string]any{
		"ids":   []any{"8d944aca-899b-4e26-9f3f-f536971c2c67"},
		"email": "alice@example.com",
	}, s, "body")
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %#v", errs)
	}
	body := normalized.(map[string]any)
	ids := body["ids"].([]any)
	if ids[0] != "8d944aca-899b-4e26-9f3f-f536971c2c67" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestValidate_StringEnumIsStrictlyString(t *testing.T) {
	s := &Schema{Type: TypeString, Enum: []any{"1"}}

	if _, errs := Validate("1", s, "body.status"); len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %#v", errs)
	}
	if _, errs := Validate("2", s, "body.status"); len(errs) != 1 || errs[0].Constraint != "enum" {
		t.Fatalf("expected enum error, got %#v", errs)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
