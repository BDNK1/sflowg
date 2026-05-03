package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
)

type AsyncTask struct {
	stepID    string
	done      chan struct{}
	result    any
	originErr *FlowError
	cancel    context.CancelFunc
	span      trace.Span
	awaited   atomic.Bool
	complete  sync.Once
}

func NewAsyncTask(stepID string, cancel context.CancelFunc, span trace.Span) *AsyncTask {
	return &AsyncTask{
		stepID: stepID,
		done:   make(chan struct{}),
		cancel: cancel,
		span:   span,
	}
}

func NewCompletedAsyncTask(stepID string, result any, err *FlowError) *AsyncTask {
	task := NewAsyncTask(stepID, func() {}, nil)
	task.Complete(result, err)
	return task
}

func (t *AsyncTask) Await(ctx context.Context, consumerStepID string) (any, *FlowError) {
	if t == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	t.awaited.Store(true)
	select {
	case <-t.done:
		return t.awaitedResult(consumerStepID)
	default:
	}
	select {
	case <-t.done:
	case <-ctx.Done():
		return nil, flowErrorForCanceledAwait(ctx.Err(), t.stepID, consumerStepID)
	}
	return t.awaitedResult(consumerStepID)
}

func (t *AsyncTask) awaitedResult(consumerStepID string) (any, *FlowError) {
	if t.originErr == nil {
		return t.result, nil
	}
	return nil, cloneFlowErrorForAwait(t.originErr, t.stepID, consumerStepID)
}

func flowErrorForCanceledAwait(err error, asyncStepID string, consumerStepID string) *FlowError {
	if err == nil {
		err = context.Canceled
	}
	code := string(ErrorCodeContextCancelled)
	if errors.Is(err, context.DeadlineExceeded) {
		code = string(ErrorCodeDeadlineExceeded)
	}
	return &FlowError{
		Type:        ErrorTypeTimeout,
		Code:        code,
		Message:     err.Error(),
		Step:        consumerStepID,
		Cause:       err,
		AwaitedFrom: asyncStepID,
	}
}

func (t *AsyncTask) Complete(result any, err *FlowError) {
	if t == nil {
		return
	}
	t.complete.Do(func() {
		t.result = result
		t.originErr = err
		close(t.done)
		if t.span != nil {
			t.span.End()
		}
	})
}

func (t *AsyncTask) Cancel() {
	if t == nil || t.cancel == nil {
		return
	}
	t.cancel()
}

func (t *AsyncTask) Awaited() bool {
	return t != nil && t.awaited.Load()
}

func (t *AsyncTask) SpanContext() trace.SpanContext {
	if t == nil || t.span == nil {
		return trace.SpanContext{}
	}
	return t.span.SpanContext()
}

type AsyncTaskRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*AsyncTask
}

func NewAsyncTaskRegistry() *AsyncTaskRegistry {
	return &AsyncTaskRegistry{tasks: make(map[string]*AsyncTask)}
}

func (r *AsyncTaskRegistry) Register(stepID string, task *AsyncTask) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[stepID] = task
}

func (r *AsyncTaskRegistry) Get(stepID string) (*AsyncTask, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[stepID]
	return task, ok
}

func (r *AsyncTaskRegistry) Snapshot() map[string]*AsyncTask {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*AsyncTask, len(r.tasks))
	for stepID, task := range r.tasks {
		out[stepID] = task
	}
	return out
}

func (r *AsyncTaskRegistry) CancelAll() {
	for _, task := range r.Snapshot() {
		task.Cancel()
	}
}
