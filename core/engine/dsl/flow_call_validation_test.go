package dsl

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/schema"
)

func TestExtractLiteralFlowCalls_StaticArgs(t *testing.T) {
	refs, err := ExtractLiteralFlowCalls(`flow.call("sub", {x: 1, name: "a", ok: true})`)
	if err != nil {
		t.Fatalf("ExtractLiteralFlowCalls() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs len = %d, want 1", len(refs))
	}
	ref := refs[0]
	if ref.TargetName != "sub" || !ref.ArgsStatic {
		t.Fatalf("ref = %#v", ref)
	}
	if _, ok := ref.ArgKeys["x"]; !ok {
		t.Fatalf("missing x key: %#v", ref.ArgKeys)
	}
	if ref.ArgValues["x"].Kind != LiteralInteger || ref.ArgValues["name"].Kind != LiteralString || ref.ArgValues["ok"].Kind != LiteralBoolean {
		t.Fatalf("literal values = %#v", ref.ArgValues)
	}
}

func TestExtractLiteralFlowCalls_IgnoresCommentsAndStrings(t *testing.T) {
	refs, err := ExtractLiteralFlowCalls(`
// flow.call("commented", {x: 1})
let text = "flow.call(\"string\", {x: 1})"
flow.call("real", {x: 1})
`)
	if err != nil {
		t.Fatalf("ExtractLiteralFlowCalls() error = %v", err)
	}
	if len(refs) != 1 || refs[0].TargetName != "real" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestExtractLiteralFlowCalls_DynamicArgsAndTarget(t *testing.T) {
	refs, err := ExtractLiteralFlowCalls(`
flow.call("literal_target", args)
flow.call(target, {x: 1})
`)
	if err != nil {
		t.Fatalf("ExtractLiteralFlowCalls() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs len = %d, want 2", len(refs))
	}
	if refs[0].TargetName != "literal_target" || refs[0].ArgsStatic {
		t.Fatalf("first ref = %#v", refs[0])
	}
	if refs[1].TargetName != "" || !refs[1].ArgsStatic {
		t.Fatalf("second ref = %#v", refs[1])
	}
}

func TestFlowValidatorRejectsMissingTargetAndCycles(t *testing.T) {
	validator := NewFlowValidator()
	err := validator.ValidateFlows(map[string]runtime.Flow{
		"a": {ID: "a", Entrypoint: runtime.Entrypoint{Type: "http"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("missing", {})`}}},
	})
	if err == nil || !strings.Contains(err.Error(), `undefined subflow "missing"`) {
		t.Fatalf("expected missing target error, got %v", err)
	}

	err = validator.ValidateFlows(map[string]runtime.Flow{
		"a": {ID: "a", Entrypoint: runtime.Entrypoint{Type: "flow"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("b", {})`}}},
		"b": {ID: "b", Entrypoint: runtime.Entrypoint{Type: "flow"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("a", {})`}}},
	})
	if err == nil || !strings.Contains(err.Error(), "flow.call cycle detected") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestFlowValidatorRejectsHTTP_TargetAndMissingRequiredLiteralArg(t *testing.T) {
	validator := NewFlowValidator()
	err := validator.ValidateFlows(map[string]runtime.Flow{
		"caller":      {ID: "caller", Entrypoint: runtime.Entrypoint{Type: "http"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("http_target", {})`}}},
		"http_target": {ID: "http_target", Entrypoint: runtime.Entrypoint{Type: "http"}},
	})
	if err == nil || !strings.Contains(err.Error(), `must be entrypoint.flow, got "http"`) {
		t.Fatalf("expected http target error, got %v", err)
	}

	contract := runtime.NewInputContract()
	contract.SetFields("input", map[string]*schema.Schema{
		"x": {Type: schema.TypeInteger, Required: true},
	})
	err = validator.ValidateFlows(map[string]runtime.Flow{
		"caller": {ID: "caller", Entrypoint: runtime.Entrypoint{Type: "http"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("sub", {})`}}},
		"sub":    {ID: "sub", Entrypoint: runtime.Entrypoint{Type: "flow", Input: contract}},
	})
	if err == nil || !strings.Contains(err.Error(), `missing required arg "x"`) {
		t.Fatalf("expected missing required arg error, got %v", err)
	}
}

func TestFlowValidatorRejectsSimpleLiteralTypeMismatch(t *testing.T) {
	contract := runtime.NewInputContract()
	contract.SetFields("input", map[string]*schema.Schema{
		"x": {Type: schema.TypeInteger, Required: true},
	})
	err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{
		"caller": {ID: "caller", Entrypoint: runtime.Entrypoint{Type: "http"}, Steps: []runtime.Step{{ID: "call", Body: `flow.call("sub", {x: "bad"})`}}},
		"sub":    {ID: "sub", Entrypoint: runtime.Entrypoint{Type: "flow", Input: contract}},
	})
	if err == nil || !strings.Contains(err.Error(), "expected integer literal") {
		t.Fatalf("expected literal type error, got %v", err)
	}
}
