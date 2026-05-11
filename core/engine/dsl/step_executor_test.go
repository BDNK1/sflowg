package dsl

import (
	"testing"

	"github.com/BDNK1/sflowg/core"
)

func newCompiledExecution() *core.Execution {
	flow := &core.Flow{ID: "payments", DSLMode: core.DSLExecutionModeCompiled}
	return core.NewExecution(flow, core.NewContainer(core.NewLogger(nil)), nil, core.NewValueStore())
}

func TestExecuteOnErrorHandler_CompiledModeFailsWhenBytecodeMissing(t *testing.T) {
	exec := newCompiledExecution()
	exec.Flow.OnErrorBody = `response.json({status: 500})`

	err := NewStepExecutor().ExecuteOnErrorHandler(exec, exec.Flow.OnErrorBody, &core.FlowError{
		Type:    core.ErrorTypePermanent,
		Code:    "FAIL",
		Message: "boom",
	})
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}

	flowErr, ok := err.(*core.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if flowErr.Code != string(core.ErrorCodeRuntimeError) {
		t.Fatalf("expected runtime error code, got %#v", flowErr)
	}
}

func TestExecuteCompensation_CompiledModeFailsWhenBytecodeMissing(t *testing.T) {
	exec := newCompiledExecution()

	err := NewStepExecutor().ExecuteCompensation(exec, `log.info("undo")`, "charge", core.SuccessPathPrimary, nil)
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}

	flowErr, ok := err.(*core.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if flowErr.Code != string(core.ErrorCodeRuntimeError) {
		t.Fatalf("expected runtime error code, got %#v", flowErr)
	}
}
