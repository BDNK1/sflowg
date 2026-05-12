package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
)

type AsyncTask struct {
	stepID           string
	done             chan struct{}
	result           any
	originErr        *FlowError
	cancel           context.CancelFunc
	span             trace.Span
	awaited          atomic.Bool
	detachedReported atomic.Bool
	complete         sync.Once
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

func (t *AsyncTask) markDetachedReported() bool {
	return t != nil && t.detachedReported.CompareAndSwap(false, true)
}

type AsyncScope struct {
	parent *AsyncScope
	mu     sync.RWMutex
	tasks  map[string]*AsyncTask
}

type AsyncTaskRegistry = AsyncScope

func NewAsyncScope(parent *AsyncScope) *AsyncScope {
	return &AsyncScope{parent: parent, tasks: make(map[string]*AsyncTask)}
}

func NewAsyncTaskRegistry() *AsyncTaskRegistry {
	return NewAsyncScope(nil)
}

func (r *AsyncScope) Register(stepID string, task *AsyncTask) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[stepID] = task
}

func (r *AsyncScope) Get(stepID string) (*AsyncTask, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	task, ok := r.tasks[stepID]
	r.mu.RUnlock()
	if ok {
		return task, true
	}
	if r.parent != nil {
		return r.parent.Get(stepID)
	}
	return task, ok
}

func (r *AsyncScope) Snapshot() map[string]*AsyncTask {
	return r.SnapshotVisible()
}

func (r *AsyncScope) SnapshotVisible() map[string]*AsyncTask {
	if r == nil {
		return nil
	}
	out := make(map[string]*AsyncTask)
	if r.parent != nil {
		for stepID, task := range r.parent.SnapshotVisible() {
			out[stepID] = task
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for stepID, task := range r.tasks {
		out[stepID] = task
	}
	return out
}

func (r *AsyncScope) CancelAll() {
	r.CancelLocal()
}

func (r *AsyncScope) CancelLocal() {
	for _, task := range r.localSnapshot() {
		task.Cancel()
	}
}

func (r *AsyncScope) WaitLocal(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, task := range r.localSnapshot() {
		if task == nil {
			continue
		}
		select {
		case <-task.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *AsyncScope) DetachLocal(supervisor *DetachedAsyncSupervisor) {
	if supervisor == nil {
		return
	}
	for _, task := range r.localSnapshot() {
		supervisor.Register(task)
	}
}

func (r *AsyncScope) localSnapshot() map[string]*AsyncTask {
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

type DetachedAsyncSupervisor struct {
	mu    sync.Mutex
	tasks []*AsyncTask
}

func NewDetachedAsyncSupervisor() *DetachedAsyncSupervisor {
	return &DetachedAsyncSupervisor{}
}

func (s *DetachedAsyncSupervisor) Register(task *AsyncTask) {
	if s == nil || task == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
}

func (s *DetachedAsyncSupervisor) CancelAll() {
	for _, task := range s.snapshot() {
		task.Cancel()
	}
}

func (s *DetachedAsyncSupervisor) StartFailureLogger(flowFinalized <-chan struct{}, logger Logger, metrics *Metrics, flowID string) {
	if s == nil || flowFinalized == nil {
		return
	}
	go func() {
		<-flowFinalized
		for _, task := range s.snapshot() {
			go recordDetachedAsyncFailure(task, logger, metrics, flowID)
		}
	}()
}

func (s *DetachedAsyncSupervisor) snapshot() []*AsyncTask {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*AsyncTask, len(s.tasks))
	copy(out, s.tasks)
	return out
}

func recordDetachedAsyncFailure(task *AsyncTask, logger Logger, metrics *Metrics, flowID string) {
	if task == nil {
		return
	}
	<-task.done
	if task.originErr == nil || task.Awaited() || !task.markDetachedReported() {
		return
	}
	logger.Error("Detached async step failed", "step", task.stepID, "error", task.originErr)
	if metrics != nil {
		metrics.RecordDetachedAsyncFailure(context.Background(), flowID, task.stepID, task.originErr.Code)
	}
}
