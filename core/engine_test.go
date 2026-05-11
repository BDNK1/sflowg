package core

import "testing"

func TestBuildStepInput_CompiledModeUsesBoundedKeys(t *testing.T) {
	flow := &Flow{ID: "payments", DSLMode: DSLExecutionModeCompiled}
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())
	exec.State().Store().Set("request", map[string]any{"body": map[string]any{"email": "alice@example.com"}})
	exec.State().Store().Set("unused", "ignore-me")

	input, err := BuildStepInput(exec, Step{
		ID:        "charge",
		Body:      `request.body.email`,
		StoreKeys: []string{"request"},
	}, SuccessPathPrimary)
	if err != nil {
		t.Fatalf("BuildStepInput() error = %v", err)
	}
	if len(input.Input) != 1 {
		t.Fatalf("expected bounded snapshot, got %#v", input.Input)
	}
	if _, ok := input.Input["request"]; !ok {
		t.Fatalf("expected request key in bounded snapshot, got %#v", input.Input)
	}
	if _, ok := input.Input["unused"]; ok {
		t.Fatalf("did not expect unused key in bounded snapshot, got %#v", input.Input)
	}
}

func TestBuildStepInput_CompiledModeAllowsEmptyStoreKeys(t *testing.T) {
	flow := &Flow{ID: "payments", DSLMode: DSLExecutionModeCompiled}
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())
	exec.State().Store().Set("request", map[string]any{"body": map[string]any{"email": "alice@example.com"}})

	input, err := BuildStepInput(exec, Step{
		ID:        "constant",
		Body:      `42`,
		StoreKeys: []string{},
	}, SuccessPathPrimary)
	if err != nil {
		t.Fatalf("BuildStepInput() error = %v", err)
	}
	if len(input.Input) != 0 {
		t.Fatalf("expected empty bounded snapshot, got %#v", input.Input)
	}
}

func TestBuildStepInput_CompiledModeFailsWhenStoreKeysMissing(t *testing.T) {
	flow := &Flow{ID: "payments", DSLMode: DSLExecutionModeCompiled}
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())

	_, err := BuildStepInput(exec, Step{
		ID:   "charge",
		Body: `request.body.email`,
	}, SuccessPathPrimary)
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}
}

func TestBuildStepInput_InterpretedModeKeepsFullSnapshot(t *testing.T) {
	flow := &Flow{ID: "payments", DSLMode: DSLExecutionModeInterpreted}
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())
	exec.State().Store().Set("request", map[string]any{"body": map[string]any{"email": "alice@example.com"}})
	exec.State().Store().Set("unused", "keep-me")

	input, err := BuildStepInput(exec, Step{
		ID:        "charge",
		Body:      `request.body.email`,
		StoreKeys: []string{"request"},
	}, SuccessPathPrimary)
	if err != nil {
		t.Fatalf("BuildStepInput() error = %v", err)
	}
	if len(input.Input) != 2 {
		t.Fatalf("expected full snapshot in interpreted mode, got %#v", input.Input)
	}
	if _, ok := input.Input["unused"]; !ok {
		t.Fatalf("expected interpreted mode to preserve full snapshot, got %#v", input.Input)
	}
}
