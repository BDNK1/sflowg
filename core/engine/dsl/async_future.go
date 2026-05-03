package dsl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/object"
	"github.com/deepnoodle-ai/risor/v2/pkg/op"
)

type asyncFutureObject struct {
	execution      *runtime.Execution
	task           *runtime.AsyncTask
	asyncStepID    string
	consumerStepID string
	errs           *asyncFutureErrors
}

func newAsyncFutureObject(execution *runtime.Execution, asyncStepID string, task *runtime.AsyncTask, consumerStepID string) *asyncFutureObject {
	return newAsyncFutureObjectWithErrors(execution, asyncStepID, task, consumerStepID, nil)
}

func newAsyncFutureObjectWithErrors(execution *runtime.Execution, asyncStepID string, task *runtime.AsyncTask, consumerStepID string, errs *asyncFutureErrors) *asyncFutureObject {
	if consumerStepID == "" {
		consumerStepID = "expression"
	}
	if errs == nil {
		errs = &asyncFutureErrors{}
	}
	return &asyncFutureObject{
		execution:      execution,
		task:           task,
		asyncStepID:    asyncStepID,
		consumerStepID: consumerStepID,
		errs:           errs,
	}
}

func applyAsyncFutures(env map[string]any, execution *runtime.Execution, consumerStepID string) map[string]any {
	applyAsyncFuturesWithErrors(env, execution, consumerStepID)
	return env
}

func applyAsyncFuturesWithErrors(env map[string]any, execution *runtime.Execution, consumerStepID string) *asyncFutureErrors {
	errs := &asyncFutureErrors{}
	if execution == nil {
		return errs
	}
	for stepID, task := range execution.AsyncTasks().Snapshot() {
		env[stepID] = newAsyncFutureObjectWithErrors(execution, stepID, task, consumerStepID, errs)
	}
	return errs
}

type asyncFutureErrors struct {
	mu  sync.Mutex
	err *runtime.FlowError
}

func (e *asyncFutureErrors) Record(fe *runtime.FlowError) {
	if e == nil || fe == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err == nil {
		e.err = fe
	}
}

func (e *asyncFutureErrors) Err() *runtime.FlowError {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

func asyncFutureError(globals map[string]any) *runtime.FlowError {
	for _, value := range globals {
		if future, ok := value.(*asyncFutureObject); ok {
			if fe := future.errs.Err(); fe != nil {
				return fe
			}
		}
	}
	return nil
}

func (o *asyncFutureObject) Type() object.Type {
	return object.Type("async_future")
}

func (o *asyncFutureObject) Inspect() string {
	return fmt.Sprintf("<async_future %s>", o.asyncStepID)
}

func (o *asyncFutureObject) Interface() interface{} {
	value, err := o.await(o.awaitContext())
	if err != nil {
		o.errs.Record(err)
		return err
	}
	return value
}

func (o *asyncFutureObject) Equals(other object.Object) bool {
	return o == other
}

func (o *asyncFutureObject) Attrs() []object.AttrSpec {
	return nil
}

func (o *asyncFutureObject) GetAttr(name string) (object.Object, bool) {
	return object.NewDynamicAttr(name, func(ctx context.Context, attrName string) (object.Object, error) {
		return o.resolveAttr(ctx, attrName)
	}), true
}

func (o *asyncFutureObject) resolveAttr(ctx context.Context, name string) (object.Object, error) {
	value, fe := o.await(ctx)
	if fe != nil {
		return nil, fe
	}
	if value == nil {
		return object.Nil, nil
	}
	obj := anyToObject(value)
	if obj == nil || obj == object.Nil {
		return object.Nil, nil
	}
	attr, ok := obj.GetAttr(name)
	if !ok {
		return object.Nil, nil
	}
	return attr, nil
}

func (o *asyncFutureObject) SetAttr(name string, value object.Object) error {
	return fmt.Errorf("async future does not support setting attribute %q", name)
}

func (o *asyncFutureObject) IsTruthy() bool {
	value, fe := o.await(o.awaitContext())
	if fe != nil {
		o.errs.Record(fe)
		return false
	}
	return value != nil
}

func (o *asyncFutureObject) RunOperation(opType op.BinaryOpType, right object.Object) (object.Object, error) {
	value, fe := o.await(o.awaitContext())
	if fe != nil {
		return nil, fe
	}
	obj := anyToObject(value)
	if obj == nil {
		obj = object.Nil
	}
	return obj.RunOperation(opType, right)
}

func (o *asyncFutureObject) await(ctx context.Context) (any, *runtime.FlowError) {
	if o.task == nil {
		return nil, nil
	}
	start := time.Now()
	value, fe := o.task.Await(ctx, o.consumerStepID)
	if o.execution != nil {
		o.execution.Metrics().RecordAsyncWait(o.execution, execFlowIDForDSL(o.execution), o.consumerStepID, o.asyncStepID, time.Since(start))
	}
	return value, fe
}

func (o *asyncFutureObject) awaitContext() context.Context {
	if o.execution == nil {
		return context.Background()
	}
	return o.execution
}

func execFlowIDForDSL(execution *runtime.Execution) string {
	if execution == nil || execution.Flow == nil {
		return ""
	}
	return execution.Flow.ID
}
