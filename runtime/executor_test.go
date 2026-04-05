package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type scriptedStepExecutor struct {
	runStep         func(ctx context.Context, execution *Execution, step Step) (string, error)
	runOnError      func(execution *Execution, body string, fe *FlowError) error
	runCompensation func(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error
}

func (s *scriptedStepExecutor) ExecuteStep(ctx context.Context, execution *Execution, step Step) (string, error) {
	if s.runStep == nil {
		return "", nil
	}
	return s.runStep(ctx, execution, step)
}

func (s *scriptedStepExecutor) ExecuteOnErrorHandler(execution *Execution, body string, fe *FlowError) error {
	if s.runOnError == nil {
		return nil
	}
	return s.runOnError(execution, body, fe)
}

func (s *scriptedStepExecutor) ExecuteCompensation(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error {
	if s.runCompensation == nil {
		return nil
	}
	return s.runCompensation(execution, body, stepID, path, compiled)
}

func newExecutorTestHarness(t *testing.T, flow *Flow, stepExecutor StepExecutor) (*Execution, *Executor) {
	t.Helper()
	exec := NewExecution(flow, NewContainer(NewLogger(nil)), nil, NewValueStore())
	executor := NewExecutor(noopEvaluator{}, stepExecutor, newIsolatedTestStepRunner(stepExecutor))
	return exec, executor
}

func TestExecuteSteps_MergesOutputAfterSuccess(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().Store().Set(step.ID, map[string]any{"ok": true})
			return "", nil
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge"}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	got, ok := exec.State().Store().Get("charge.ok")
	if !ok || got != true {
		t.Fatalf("expected merged result charge.ok=true, got %v (present=%v)", got, ok)
	}
}

func TestExecuteSteps_FailedAttemptDoesNotLeakResult(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().Store().Set(step.ID, "dirty")
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge"}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}

	if got, ok := exec.State().Store().Get("charge"); ok {
		t.Fatalf("expected no leaked result, got %v", got)
	}
}

func TestExecuteSteps_AccumulatesSideEffectsAfterSuccess(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().RecordSideEffect(SideEffect{Plugin: "wallet", Method: step.ID})
			execution.State().Store().Set(step.ID, map[string]any{"ok": true})
			return "", nil
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge"}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected 1 side effect, got %#v", effects)
	}
	if effects[0].Plugin != "wallet" || effects[0].Method != "charge" {
		t.Fatalf("unexpected side effect %#v", effects[0])
	}
}

func TestExecuteSteps_FailedAttemptDoesNotLeakSideEffects(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().RecordSideEffect(SideEffect{Plugin: "wallet", Method: "charge"})
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge"}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}
	if effects := exec.State().SideEffects(); len(effects) != 0 {
		t.Fatalf("expected no leaked side effects, got %#v", effects)
	}
}

func TestExecuteSteps_RetryMergesOnlyWinningAttemptSideEffects(t *testing.T) {
	attempts := 0
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			attempts++
			execution.State().RecordSideEffect(SideEffect{Plugin: "wallet", Method: fmt.Sprintf("attempt-%d", attempts)})
			if attempts == 1 {
				return "", &FlowError{Type: ErrorTypeTransient, Code: "RETRY", Message: "retry", Step: step.ID}
			}
			execution.State().Store().Set(step.ID, map[string]any{"ok": true})
			return "", nil
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge", Retry: &RetryConfig{MaxAttempts: 2}}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected only winning attempt side effect, got %#v", effects)
	}
	if effects[0].Method != "attempt-2" {
		t.Fatalf("expected second attempt side effect, got %#v", effects[0])
	}
}

func TestExecuteSteps_FailedAttemptDoesNotLeakResponse(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge"}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}

	if got := exec.State().Response(); got != nil {
		t.Fatalf("expected no leaked response, got %#v", got)
	}
}

func TestExecuteSteps_FailedAttemptDoesNotLeakNext(t *testing.T) {
	attempts := 0
	visited := []string{}
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			visited = append(visited, step.ID)
			if step.ID != "start" {
				return "", nil
			}
			attempts++
			if attempts == 1 {
				return "finish", &FlowError{Type: ErrorTypeTransient, Code: "RETRY", Message: "retry", Step: step.ID}
			}
			return "", nil
		},
	}
	flow := &Flow{
		ID: "payments",
		Steps: []Step{
			{ID: "start", Retry: &RetryConfig{MaxAttempts: 2}},
			{ID: "middle"},
			{ID: "finish"},
		},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	want := []string{"start", "start", "middle", "finish"}
	if fmt.Sprintf("%v", visited) != fmt.Sprintf("%v", want) {
		t.Fatalf("expected visited=%v, got %v", want, visited)
	}
}

func TestExecuteSteps_RetryGetsCleanSnapshot(t *testing.T) {
	attempts := 0
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			attempts++
			if _, ok := execution.State().Store().Get(step.ID); ok {
				return "", &FlowError{Type: ErrorTypePermanent, Code: "LEAK", Message: "state leaked across retries", Step: step.ID}
			}
			if attempts == 1 {
				execution.State().Store().Set(step.ID, "dirty")
				return "", &FlowError{Type: ErrorTypeTransient, Code: "RETRY", Message: "retry", Step: step.ID}
			}
			return "", nil
		},
	}
	flow := &Flow{ID: "payments", Steps: []Step{{ID: "charge", Retry: &RetryConfig{MaxAttempts: 2}}}}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestExecuteSteps_FallbackOutputMergesCorrectly(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			if step.Body == "fallback" {
				execution.State().Store().Set(step.ID, map[string]any{"mode": "fallback"})
				return "", nil
			}
			return "", &FlowError{Type: ErrorTypePermanent, Code: "PRIMARY_FAILED", Message: "primary failed", Step: step.ID}
		},
	}
	flow := &Flow{
		ID:    "payments",
		Steps: []Step{{ID: "charge", Body: "primary", FallbackBody: "fallback"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	got, ok := exec.State().Store().Get("charge.mode")
	if !ok || got != "fallback" {
		t.Fatalf("expected fallback result under original step ID, got %v (present=%v)", got, ok)
	}
}

func TestExecuteSteps_FallbackMergesFallbackSideEffects(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			if step.Body == "fallback" {
				execution.State().RecordSideEffect(SideEffect{Plugin: "wallet", Method: "fallback"})
				execution.State().Store().Set(step.ID, map[string]any{"mode": "fallback"})
				return "", nil
			}
			execution.State().RecordSideEffect(SideEffect{Plugin: "wallet", Method: "primary"})
			return "", &FlowError{Type: ErrorTypePermanent, Code: "PRIMARY_FAILED", Message: "primary failed", Step: step.ID}
		},
	}
	flow := &Flow{
		ID:    "payments",
		Steps: []Step{{ID: "charge", Body: "primary", FallbackBody: "fallback"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected only fallback side effect, got %#v", effects)
	}
	if effects[0].Method != "fallback" {
		t.Fatalf("expected fallback side effect, got %#v", effects[0])
	}
}

func TestExecuteSteps_ResponseCausesEarlyExit(t *testing.T) {
	visited := []string{}
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			visited = append(visited, step.ID)
			if step.ID == "respond" {
				execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			}
			return "", nil
		},
	}
	flow := &Flow{
		ID:    "payments",
		Steps: []Step{{ID: "respond"}, {ID: "after"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	want := []string{"respond"}
	if fmt.Sprintf("%v", visited) != fmt.Sprintf("%v", want) {
		t.Fatalf("expected visited=%v, got %v", want, visited)
	}
}

func TestExecuteSteps_NextDrivesBranching(t *testing.T) {
	visited := []string{}
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			visited = append(visited, step.ID)
			if step.ID == "start" {
				return "finish", nil
			}
			return "", nil
		},
	}
	flow := &Flow{
		ID:    "payments",
		Steps: []Step{{ID: "start"}, {ID: "middle"}, {ID: "finish"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	want := []string{"start", "finish"}
	if fmt.Sprintf("%v", visited) != fmt.Sprintf("%v", want) {
		t.Fatalf("expected visited=%v, got %v", want, visited)
	}
}

func TestExecuteSteps_CompensationStillWorks(t *testing.T) {
	compensated := false
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			if step.ID == "reserve" {
				return "", nil
			}
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runCompensation: func(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error {
			compensated = true
			if stepID != "reserve" || path != SuccessPathPrimary {
				return errors.New("unexpected compensation target")
			}
			return nil
		},
	}
	flow := &Flow{
		ID: "payments",
		Steps: []Step{
			{ID: "reserve", CompensateBody: "undo"},
			{ID: "charge"},
		},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}
	if !compensated {
		t.Fatal("expected compensation to run")
	}
}

func TestOnError_IsolatedExecutionMergesResponseAndStore(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runOnError: func(execution *Execution, body string, fe *FlowError) error {
			execution.State().Store().SetNested("recovery", map[string]any{"handled": true, "code": fe.Code})
			execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			return nil
		},
	}
	flow := &Flow{
		ID:          "payments",
		OnErrorBody: "recover",
		Steps:       []Step{{ID: "charge"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected on_error to handle failure, got %v", err)
	}

	if got, ok := exec.State().Store().Get("recovery.handled"); !ok || got != true {
		t.Fatalf("expected merged recovery store, got %v (present=%v)", got, ok)
	}
	if got := exec.State().Response(); got == nil || got.HandlerName != "http.json" {
		t.Fatalf("expected merged response, got %#v", got)
	}
}

func TestOnError_FailedAttemptDoesNotLeakPartialStore(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runOnError: func(execution *Execution, body string, fe *FlowError) error {
			execution.State().Store().SetNested("recovery", map[string]any{"handled": true})
			return errors.New("handler failed")
		},
	}
	flow := &Flow{
		ID:          "payments",
		OnErrorBody: "recover",
		Steps:       []Step{{ID: "charge"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected on_error failure")
	}
	if _, ok := exec.State().Store().Get("recovery"); ok {
		t.Fatal("expected failed on_error changes to remain isolated")
	}
}

func TestOnError_MergesSideEffects(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runOnError: func(execution *Execution, body string, fe *FlowError) error {
			execution.State().RecordSideEffect(SideEffect{Plugin: "notify", Method: "send"})
			return nil
		},
	}
	flow := &Flow{
		ID:          "payments",
		OnErrorBody: "recover",
		Steps:       []Step{{ID: "charge"}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("expected on_error to handle failure, got %v", err)
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected merged on_error side effect, got %#v", effects)
	}
	if effects[0].Plugin != "notify" || effects[0].Method != "send" {
		t.Fatalf("unexpected side effect %#v", effects[0])
	}
}

func TestCompensation_IsolatedExecutionMergesSideEffectsOnly(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			if step.ID == "reserve" {
				execution.State().Store().Set(step.ID, map[string]any{"ok": true})
				return "", nil
			}
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runCompensation: func(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error {
			execution.State().RecordSideEffect(SideEffect{Plugin: "inventory", Method: "release"})
			execution.State().Store().SetNested("compensation", map[string]any{"changed": true})
			execution.State().SetResponse(&ResponseDescriptor{HandlerName: "http.json"})
			return nil
		},
	}
	flow := &Flow{
		ID: "payments",
		Steps: []Step{
			{ID: "reserve", CompensateBody: "undo"},
			{ID: "charge"},
		},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}

	effects := exec.State().SideEffects()
	if len(effects) != 1 {
		t.Fatalf("expected merged compensation side effect, got %#v", effects)
	}
	if effects[0].Plugin != "inventory" || effects[0].Method != "release" {
		t.Fatalf("unexpected compensation side effect %#v", effects[0])
	}
	if _, ok := exec.State().Store().Get("compensation"); ok {
		t.Fatal("expected compensation store changes to remain isolated")
	}
	if exec.State().Response() != nil {
		t.Fatalf("expected compensation response not to merge, got %#v", exec.State().Response())
	}
}

func TestCompensation_FailureDoesNotLeakPartialState(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			if step.ID == "reserve" {
				execution.State().Store().Set(step.ID, map[string]any{"ok": true})
				return "", nil
			}
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
		},
		runCompensation: func(execution *Execution, body string, stepID string, path SuccessPath, compiled any) error {
			execution.State().RecordSideEffect(SideEffect{Plugin: "inventory", Method: "release"})
			execution.State().Store().SetNested("compensation", map[string]any{"changed": true})
			return errors.New("compensation failed")
		},
	}
	flow := &Flow{
		ID: "payments",
		Steps: []Step{
			{ID: "reserve", CompensateBody: "undo"},
			{ID: "charge"},
		},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err == nil {
		t.Fatal("expected flow error")
	}
	if effects := exec.State().SideEffects(); len(effects) != 0 {
		t.Fatalf("expected failed compensation side effects to remain isolated, got %#v", effects)
	}
	if _, ok := exec.State().Store().Get("compensation"); ok {
		t.Fatal("expected failed compensation store changes to remain isolated")
	}
}
