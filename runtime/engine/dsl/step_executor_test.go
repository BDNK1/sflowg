package dsl

import (
	"testing"

	"github.com/BDNK1/sflowg/runtime"
)

func newCompiledExecution() *runtime.Execution {
	flow := &runtime.Flow{ID: "payments", DSLMode: runtime.DSLExecutionModeCompiled}
	return runtime.NewExecution(flow, runtime.NewContainer(runtime.NewLogger(nil)), nil, runtime.NewValueStore())
}

func TestExecuteOnErrorHandler_CompiledModeFailsWhenBytecodeMissing(t *testing.T) {
	exec := newCompiledExecution()
	exec.Flow.OnErrorBody = `response.json({status: 500})`

	err := NewStepExecutor().ExecuteOnErrorHandler(exec, exec.Flow.OnErrorBody, &runtime.FlowError{
		Type:    runtime.ErrorTypePermanent,
		Code:    "FAIL",
		Message: "boom",
	})
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}

	flowErr, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if flowErr.Code != string(runtime.ErrorCodeRuntimeError) {
		t.Fatalf("expected runtime error code, got %#v", flowErr)
	}
}

func TestExecuteCompensation_CompiledModeFailsWhenBytecodeMissing(t *testing.T) {
	exec := newCompiledExecution()

	err := NewStepExecutor().ExecuteCompensation(exec, `log.info("undo")`, "charge", runtime.SuccessPathPrimary, nil)
	if err == nil {
		t.Fatal("expected compiled mode invariant error, got nil")
	}

	flowErr, ok := err.(*runtime.FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T (%v)", err, err)
	}
	if flowErr.Code != string(runtime.ErrorCodeRuntimeError) {
		t.Fatalf("expected runtime error code, got %#v", flowErr)
	}
}
