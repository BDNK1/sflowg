package runtime

import (
	"errors"
	"testing"
)

type sideEffectWrapperPlugin struct{}

func (p *sideEffectWrapperPlugin) Charge(_ *Execution, args map[string]any) (map[string]any, error) {
	return map[string]any{
		"status":  "captured",
		"amount":  args["amount"],
		"receipt": map[string]any{"id": "rcpt_123"},
	}, nil
}

func (p *sideEffectWrapperPlugin) Fail(_ *Execution, args map[string]any) (map[string]any, error) {
	return nil, errors.New("upstream failure")
}

func TestSummarizeSideEffectInput_BoundsNestedPayloads(t *testing.T) {
	summary := summarizeSideEffectInput(map[string]any{
		"amount": 42,
		"meta": map[string]any{
			"customer": "cus_123",
			"card":     map[string]any{"last4": "4242"},
		},
		"items": []any{1, 2, 3},
	})

	result, ok := summary.(map[string]any)
	if !ok {
		t.Fatalf("expected map summary, got %#v", summary)
	}
	if result["amount"] != 42 {
		t.Fatalf("expected scalar value to be preserved, got %#v", result["amount"])
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok || meta["type"] != "object" || meta["size"] != 2 {
		t.Fatalf("expected bounded object summary, got %#v", result["meta"])
	}

	items, ok := result["items"].(map[string]any)
	if !ok || items["type"] != "array" || items["size"] != 3 {
		t.Fatalf("expected bounded array summary, got %#v", result["items"])
	}
}

func TestPluginTaskWrapper_RecordsPluginMethodAndSummaries(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("wallet", &sideEffectWrapperPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	exec := NewExecution(&Flow{ID: "payments"}, container, nil, NewValueStore())
	task := container.GetTask("wallet.charge")
	if task == nil {
		t.Fatal("expected task registration")
	}

	_, err := task.Execute(exec, map[string]any{
		"amount": 42,
		"meta":   map[string]any{"customer": "cus_123"},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected 1 side effect, got %#v", effects)
	}

	effect := effects[0]
	if effect.Plugin != "wallet" || effect.Method != "charge" {
		t.Fatalf("unexpected side effect metadata %#v", effect)
	}

	input, ok := effect.Input.(map[string]any)
	if !ok {
		t.Fatalf("expected input summary map, got %#v", effect.Input)
	}
	if input["amount"] != 42 {
		t.Fatalf("expected scalar amount to be preserved, got %#v", input["amount"])
	}

	output, ok := effect.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected output summary map, got %#v", effect.Output)
	}
	if output["status"] != "captured" {
		t.Fatalf("expected scalar output to be preserved, got %#v", output["status"])
	}

	receipt, ok := output["receipt"].(map[string]any)
	if !ok || receipt["type"] != "object" || receipt["size"] != 1 {
		t.Fatalf("expected bounded nested output, got %#v", output["receipt"])
	}
}

func TestPluginTaskWrapper_RecordsErrorString(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("wallet", &sideEffectWrapperPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	exec := NewExecution(&Flow{ID: "payments"}, container, nil, NewValueStore())
	task := container.GetTask("wallet.fail")
	if task == nil {
		t.Fatal("expected task registration")
	}

	if _, err := task.Execute(exec, map[string]any{"amount": 42}); err == nil {
		t.Fatal("expected plugin failure")
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected 1 side effect, got %#v", effects)
	}
	if effects[0].Error != "upstream failure" {
		t.Fatalf("expected error string to be recorded, got %#v", effects[0])
	}
}
