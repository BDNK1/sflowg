package runtime

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (e *Executor) spawnAsyncStep(execution *Execution, step Step, flowFinalized <-chan struct{}) *FlowError {
	asyncRuntime := asyncRuntimeForExecution(execution)
	if asyncRuntime == nil {
		return &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeRuntimeError),
			Message: "async runtime unavailable: execution has no container",
			Step:    step.ID,
		}
	}
	if err := asyncRuntime.Acquire(execution, execution.Metrics(), execFlowID(execution), step.ID); err != nil {
		return e.flowError(execution, step.ID, err)
	}

	snapshot := execution.State().Store().Snapshot()
	taskCtx, taskCancel := context.WithCancel(asyncRuntime.Context())
	spanCtx, span := execution.Tracer().Start(taskCtx, fmt.Sprintf("async step %s", step.ID),
		trace.WithAttributes(attribute.String("step.id", step.ID)))
	task := NewAsyncTask(step.ID, taskCancel, span)
	execution.AsyncTasks().Register(step.ID, task)
	execution.Metrics().RecordAsyncSpawn(spanCtx, execFlowID(execution), step.ID)

	isolatedExec := execution.
		WithIsolatedState(cloneRunStateSnapshot(snapshot)).
		WithContext(spanCtx).
		WithActiveStep(step.ID)

	go func() {
		fe := func() *FlowError {
			defer asyncRuntime.Release()
			output, fe, _ := e.runStepPipeline(isolatedExec, step)
			if fe != nil {
				task.Complete(nil, fe)
				return fe
			}
			task.Complete(output.Result, nil)
			return nil
		}()
		if fe != nil {
			<-flowFinalized
			if !task.Awaited() && task.markDetachedReported() {
				execution.Logger().Error("Detached async step failed", "step", step.ID, "error", fe)
				execution.Metrics().RecordDetachedAsyncFailure(context.Background(), execFlowID(execution), step.ID, fe.Code)
			}
		}
	}()

	return nil
}

func (e *Executor) startAsyncCancellationWatcher(execution *Execution, flowFinalized <-chan struct{}, detachedAsync *DetachedAsyncSupervisor) {
	asyncRuntime := asyncRuntimeForExecution(execution)
	if asyncRuntime == nil {
		return
	}
	go func() {
		select {
		case <-execution.Done():
			select {
			case <-flowFinalized:
				return
			default:
			}
			execution.AsyncTasks().CancelAll()
			detachedAsync.CancelAll()
		case <-flowFinalized:
			return
		case <-asyncRuntime.Context().Done():
			detachedAsync.CancelAll()
			return
		}
	}()
}

func asyncRuntimeForExecution(execution *Execution) *AsyncRuntime {
	if execution != nil && execution.Container != nil {
		return execution.Container.AsyncRuntime()
	}
	return nil
}
