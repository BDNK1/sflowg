package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAsyncScope_LayeredLookupAndLocalSnapshot(t *testing.T) {
	parent := NewAsyncScope(nil)
	parentTask := NewCompletedAsyncTask("prefetch", "parent", nil)
	parent.Register("prefetch", parentTask)

	child := NewAsyncScope(parent)
	childTask := NewCompletedAsyncTask("enrich", "child", nil)
	child.Register("enrich", childTask)

	if task, ok := child.Get("prefetch"); !ok || task != parentTask {
		t.Fatalf("parent task lookup = %v, %v; want parent task, true", task, ok)
	}
	if task, ok := child.Get("enrich"); !ok || task != childTask {
		t.Fatalf("local task lookup = %v, %v; want child task, true", task, ok)
	}

	visible := child.SnapshotVisible()
	if len(visible) != 2 {
		t.Fatalf("visible tasks len = %d, want 2", len(visible))
	}

	supervisor := NewDetachedAsyncSupervisor()
	child.DetachLocal(supervisor)
	if len(supervisor.tasks) != 1 || supervisor.tasks[0] != childTask {
		t.Fatalf("DetachLocal should transfer only child task")
	}
}

func TestAsyncScope_CancelAllCancelsOnlyLocalTasks(t *testing.T) {
	parentCanceled := false
	childCanceled := false
	parent := NewAsyncScope(nil)
	parent.Register("parent", NewAsyncTask("parent", func() { parentCanceled = true }, nil))
	child := NewAsyncScope(parent)
	child.Register("child", NewAsyncTask("child", func() { childCanceled = true }, nil))

	child.CancelAll()

	if parentCanceled {
		t.Fatal("child CancelAll cancelled parent task")
	}
	if !childCanceled {
		t.Fatal("child CancelAll did not cancel local task")
	}
}

func TestAsyncScope_WaitLocalWaitsOnlyForLocalTasks(t *testing.T) {
	parent := NewAsyncScope(nil)
	parent.Register("parent", NewAsyncTask("parent", func() {}, nil))

	child := NewAsyncScope(parent)
	childTask := NewAsyncTask("child", func() {}, nil)
	child.Register("child", childTask)

	done := make(chan error, 1)
	go func() {
		done <- child.WaitLocal(context.Background())
	}()

	select {
	case err := <-done:
		t.Fatalf("WaitLocal returned before local task completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	childTask.Complete("ok", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitLocal returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitLocal did not return after local task completed")
	}
}

func TestDetachedAsyncSupervisor_CancelsDetachedTasks(t *testing.T) {
	canceled := false
	task := NewAsyncTask("local", func() { canceled = true }, nil)
	supervisor := NewDetachedAsyncSupervisor()
	supervisor.Register(task)

	supervisor.CancelAll()

	if !canceled {
		t.Fatal("CancelAll did not cancel detached task")
	}
}

func TestDetachedAsyncSupervisor_LogsFailuresAfterFlowFinalized(t *testing.T) {
	var buf lockedBuffer
	logger := NewLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	flowFinalized := make(chan struct{})
	task := NewCompletedAsyncTask("local", nil, &FlowError{Code: "FAIL", Message: "boom"})
	supervisor := NewDetachedAsyncSupervisor()
	supervisor.Register(task)

	supervisor.StartFailureLogger(flowFinalized, logger, nil, "flow")
	close(flowFinalized)

	deadline := time.After(time.Second)
	for {
		if strings.Contains(buf.String(), "Detached async step failed") {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("detached failure was not logged: %s", buf.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
