package runtime

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteSteps_ParallelWaitAllSuccessMergesBranchOutputs(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			execution.State().Store().Set(step.ID, map[string]any{"ok": step.ID})
			return "", nil
		},
	}
	flow := &Flow{
		ID: "payments",
		Nodes: []FlowNode{{
			ID:   "__parallel_1",
			Kind: FlowNodeParallel,
			Parallel: &ParallelBlock{Branches: []Step{
				{ID: "enrich"},
				{ID: "score"},
			}},
		}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	for _, key := range []string{"enrich.ok", "score.ok"} {
		if got, ok := exec.State().Store().Get(key); !ok || got == "" {
			t.Fatalf("expected merged key %s, got %v present=%v", key, got, ok)
		}
	}
}

func TestExecuteSteps_ParallelWaitAllMultiFailureIsStable(t *testing.T) {
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom " + step.ID, Step: step.ID}
		},
	}
	flow := &Flow{
		ID: "payments",
		Nodes: []FlowNode{{
			ID:   "__parallel_1",
			Kind: FlowNodeParallel,
			Parallel: &ParallelBlock{Branches: []Step{
				{ID: "first"},
				{ID: "second"},
			}},
		}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok {
		t.Fatalf("expected FlowError, got %T", err)
	}
	if fe.Code != string(ErrorCodeParallelFailure) || fe.Step != "__parallel_1" {
		t.Fatalf("unexpected parallel failure: %#v", fe)
	}
	if len(fe.Failures) != 2 || fe.Failures[0].Branch != "first" || fe.Failures[1].Branch != "second" {
		t.Fatalf("unexpected failures ordering: %#v", fe.Failures)
	}
}

func TestExecuteSteps_ParallelMaxInFlightLimitsConcurrency(t *testing.T) {
	var active atomic.Int32
	var maxSeen atomic.Int32
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			now := active.Add(1)
			for {
				max := maxSeen.Load()
				if now <= max || maxSeen.CompareAndSwap(max, now) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return "", nil
		},
	}
	flow := &Flow{
		ID: "payments",
		Nodes: []FlowNode{{
			ID:   "__parallel_1",
			Kind: FlowNodeParallel,
			Parallel: &ParallelBlock{
				Options: ParallelOptions{MaxInFlight: 2},
				Branches: []Step{
					{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"},
				},
			},
		}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	if err := executor.ExecuteSteps(exec); err != nil {
		t.Fatalf("ExecuteSteps() error = %v", err)
	}
	if got := maxSeen.Load(); got > 2 {
		t.Fatalf("max concurrency = %d, want <= 2", got)
	}
}

func TestExecuteSteps_ParallelFailFastMergesCompletedSuccesses(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			switch step.ID {
			case "ok":
				execution.State().Store().Set(step.ID, map[string]any{"done": true})
				once.Do(func() { close(release) })
				return "", nil
			case "fail":
				<-release
				return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
			default:
				return "", fmt.Errorf("unexpected branch %s", step.ID)
			}
		},
	}
	flow := &Flow{
		ID: "payments",
		Nodes: []FlowNode{{
			ID:   "__parallel_1",
			Kind: FlowNodeParallel,
			Parallel: &ParallelBlock{
				Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureFailFast},
				Branches: []Step{{ID: "ok"}, {ID: "fail"}},
			},
		}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok || fe.Step != "fail" {
		t.Fatalf("expected original branch failure, got %#v", err)
	}
	if got, ok := exec.State().Store().Get("ok.done"); !ok || got != true {
		t.Fatalf("expected completed success to merge, got %v present=%v", got, ok)
	}
}

func TestExecuteSteps_ParallelFailFastCancelsRegisteredAsyncBranch(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	var startedOnce sync.Once
	var canceledOnce sync.Once
	stepExecutor := &scriptedStepExecutor{
		runStep: func(ctx context.Context, execution *Execution, step Step) (string, error) {
			switch step.ID {
			case "async_work":
				startedOnce.Do(func() { close(started) })
				<-ctx.Done()
				canceledOnce.Do(func() { close(canceled) })
				return "", ctx.Err()
			case "fail":
				<-started
				return "", &FlowError{Type: ErrorTypePermanent, Code: "FAIL", Message: "boom", Step: step.ID}
			default:
				return "", nil
			}
		},
	}
	flow := &Flow{
		ID: "payments",
		Nodes: []FlowNode{{
			ID:   "__parallel_1",
			Kind: FlowNodeParallel,
			Parallel: &ParallelBlock{
				Options:  ParallelOptions{MaxInFlight: 2, OnFailure: OnFailureFailFast},
				Branches: []Step{{ID: "async_work", Async: true}, {ID: "fail"}},
			},
		}},
	}
	exec, executor := newExecutorTestHarness(t, flow, stepExecutor)

	err := executor.ExecuteSteps(exec)
	fe, ok := err.(*FlowError)
	if !ok || fe.Step != "fail" {
		t.Fatalf("expected original fail branch error, got %#v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("expected fail_fast to cancel registered async branch")
	}
}
