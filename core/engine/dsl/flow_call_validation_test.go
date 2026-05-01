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

func TestFlowValidatorDiscoversHeaderCallSubflowSugar(t *testing.T) {
	caller, err := Parse(`step call_sub as subflow.sub {
	x: 1
}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	caller.ID = "caller"
	caller.Entrypoint = runtime.Entrypoint{Type: "http"}

	sub := runtime.Flow{
		ID:         "sub",
		Entrypoint: runtime.Entrypoint{Type: "flow", Input: runtime.NewInputContract()},
	}

	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"caller": caller, "sub": sub}); err != nil {
		t.Fatalf("ValidateFlows() error = %v", err)
	}
}

func TestFlowValidatorRejectsUndefinedHeaderCallSubflowSugar(t *testing.T) {
	caller, err := Parse(`step call_sub as subflow.missing {
	x: 1
}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	caller.ID = "caller"
	caller.Entrypoint = runtime.Entrypoint{Type: "http"}

	err = NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"caller": caller})
	if err == nil || !strings.Contains(err.Error(), `undefined subflow "missing"`) {
		t.Fatalf("expected missing subflow error, got %v", err)
	}
}

func TestFlowValidatorKafkaResponseSemantics(t *testing.T) {
	valid := runtime.Flow{
		ID:               "consume",
		Entrypoint:       runtime.Entrypoint{Type: "kafka", Config: kafkaEntrypointConfig()},
		ResponseContract: runtime.KafkaResponseContract(),
		Return:           runtime.Return{Body: `response.ack()`},
		Steps:            []runtime.Step{{ID: "__return", Body: `response.ack()`}},
		OnErrorBody:      `response.nack()`,
	}
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": valid}); err != nil {
		t.Fatalf("valid kafka flow failed: %v", err)
	}

	invalidReturn := valid
	invalidReturn.Return.Body = ""
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": invalidReturn}); err == nil || !strings.Contains(err.Error(), "top-level return") {
		t.Fatalf("expected missing top-level return error, got %v", err)
	}

	invalidStep := valid
	invalidStep.Steps = []runtime.Step{{ID: "bad", Body: `response.ack()`}}
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": invalidStep}); err == nil || !strings.Contains(err.Error(), "only allowed") {
		t.Fatalf("expected step ack rejection, got %v", err)
	}

	invalidArg := valid
	invalidArg.Return.Body = `response.ack({})`
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": invalidArg}); err == nil || !strings.Contains(err.Error(), "expects no arguments") {
		t.Fatalf("expected ack arg rejection, got %v", err)
	}

	invalidSubtype := valid
	invalidSubtype.Return.Body = `response.json({})`
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": invalidSubtype}); err == nil || !strings.Contains(err.Error(), "json") {
		t.Fatalf("expected response.json rejection, got %v", err)
	}
}

func TestFlowValidatorKafkaRequiresJSONValue(t *testing.T) {
	flow := runtime.Flow{
		ID:               "consume",
		Entrypoint:       runtime.Entrypoint{Type: "kafka", Config: map[string]any{"value": map[string]any{"type": "text"}}},
		ResponseContract: runtime.KafkaResponseContract(),
		Return:           runtime.Return{Body: `response.ack()`},
		Steps:            []runtime.Step{{ID: "__return", Body: `response.ack()`}},
	}
	err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"consume": flow})
	if err == nil || !strings.Contains(err.Error(), "value.type must be json") {
		t.Fatalf("expected json value type error, got %v", err)
	}
}

func TestFlowValidatorCronResponseSemantics(t *testing.T) {
	valid := runtime.Flow{
		ID:               "reconcile",
		Entrypoint:       runtime.Entrypoint{Type: "cron", Config: map[string]any{"schedule": "*/5 * * * *"}},
		ResponseContract: runtime.CronResponseContract(),
		Steps:            []runtime.Step{{ID: "work", Body: `log.info(trigger.scheduled_at, trigger.fire_count)`}},
	}
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"reconcile": valid}); err != nil {
		t.Fatalf("valid cron flow failed: %v", err)
	}

	invalid := valid
	invalid.Return = runtime.Return{Body: `response.json({status: 200})`}
	invalid.Steps = append(invalid.Steps, runtime.Step{ID: "__return", Body: invalid.Return.Body})
	if err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"reconcile": invalid}); err == nil || !strings.Contains(err.Error(), "response.json") {
		t.Fatalf("expected cron response rejection, got %v", err)
	}
}

func TestFlowValidatorHTTPRequiresTopLevelReturn(t *testing.T) {
	flow := runtime.Flow{
		ID:               "payments",
		Entrypoint:       runtime.Entrypoint{Type: "http"},
		ResponseContract: runtime.HTTPResponseContract(),
		Steps:            []runtime.Step{{ID: "respond", Body: `response.json({status: 200})`}},
	}
	err := NewFlowValidator().ValidateFlows(map[string]runtime.Flow{"payments": flow})
	if err == nil || !strings.Contains(err.Error(), "requires top-level return") {
		t.Fatalf("expected missing top-level return error, got %v", err)
	}
}

func kafkaEntrypointConfig() map[string]any {
	return map[string]any{
		"broker":            "default",
		"topic":             "orders",
		"group_id":          "orders-service",
		"auto_offset_reset": "earliest",
		"value":             map[string]any{"type": "json"},
	}
}
