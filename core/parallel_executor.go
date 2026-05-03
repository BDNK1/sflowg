package runtime

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type parallelBranchResult struct {
	Index    int
	BranchID string
	Output   StepOutput
	Err      *FlowError
	Started  bool
	Skipped  bool
}

func (e *Executor) executeParallelBlock(execution *Execution, node FlowNode, block ParallelBlock, runCtx *executionRunContext) *FlowError {
	options := effectiveParallelOptions(execution, block.Options)
	parentCtx := execution.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	spanCtx, span := execution.Tracer().Start(parentCtx, fmt.Sprintf("parallel block %s", node.ID),
		trace.WithAttributes(
			attribute.String("parallel.block.id", node.ID),
			attribute.Int("parallel.max_in_flight", options.MaxInFlight),
			attribute.String("parallel.on_failure", string(options.OnFailure)),
		),
	)
	defer span.End()

	blockCtx, cancel := context.WithCancel(spanCtx)
	defer cancel()

	snapshot := execution.State().Store().Snapshot()
	results := make([]parallelBranchResult, len(block.Branches))
	sem := make(chan struct{}, options.MaxInFlight)
	asyncTasks := &parallelAsyncTaskTracker{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr *FlowError

	for i, branch := range block.Branches {
		i, branch := i, branch
		results[i] = parallelBranchResult{Index: i, BranchID: branch.ID}
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := e.runParallelBranch(execution, node.ID, branch, i, snapshot, sem, blockCtx, runCtx, asyncTasks)
			mu.Lock()
			results[i] = result
			if result.Err != nil && options.OnFailure == OnFailureFailFast && firstErr == nil {
				firstErr = result.Err
				cancel()
				asyncTasks.CancelAll()
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	for _, result := range results {
		if result.Err != nil {
			continue
		}
		if result.Skipped || !result.Started {
			continue
		}
		applyStepOutput(execution, result.BranchID, result.Output)
	}

	if options.OnFailure == OnFailureFailFast {
		if firstErr != nil {
			asyncTasks.CancelAll()
			asyncTasks.WaitAll()
			span.RecordError(firstErr)
			span.SetStatus(codes.Error, firstErr.Message)
		}
		return firstErr
	}

	failures := make([]FlowFailure, 0)
	var single *FlowError
	for _, result := range results {
		if result.Err == nil {
			continue
		}
		single = result.Err
		failures = append(failures, FlowFailure{
			Branch: result.BranchID,
			Step:   result.BranchID,
			Error:  FlowErrorDetailFrom(result.Err),
		})
	}
	if len(failures) == 0 {
		return nil
	}
	if len(failures) == 1 {
		span.RecordError(single)
		span.SetStatus(codes.Error, single.Message)
		return single
	}
	fe := NewParallelFailure(node.ID, failures)
	span.RecordError(fe)
	span.SetStatus(codes.Error, fe.Message)
	return fe
}

func effectiveParallelOptions(execution *Execution, options ParallelOptions) ParallelOptions {
	cfg := NormalizeParallelConfig(ParallelConfig{})
	if execution != nil && execution.Container != nil {
		cfg = execution.Container.RuntimeConfig().Parallel
	}
	if options.MaxInFlight <= 0 {
		options.MaxInFlight = cfg.BlockDefaultMaxInFlight
	}
	if options.MaxInFlight <= 0 {
		options.MaxInFlight = DefaultParallelBlockMaxInFlight
	}
	if options.OnFailure == "" {
		options.OnFailure = cfg.DefaultOnFailure
	}
	if options.OnFailure == "" {
		options.OnFailure = OnFailureWaitAll
	}
	return options
}

func (e *Executor) runParallelBranch(
	parent *Execution,
	blockID string,
	branch Step,
	index int,
	parentSnapshot map[string]any,
	sem chan struct{},
	blockCtx context.Context,
	runCtx *executionRunContext,
	asyncTasks *parallelAsyncTaskTracker,
) parallelBranchResult {
	result := parallelBranchResult{Index: index, BranchID: branch.ID}
	if err := blockCtx.Err(); err != nil {
		result.Err = e.flowError(parent, branch.ID, err)
		return result
	}
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-blockCtx.Done():
		result.Err = e.flowError(parent, branch.ID, blockCtx.Err())
		return result
	}
	if err := blockCtx.Err(); err != nil {
		result.Err = e.flowError(parent, branch.ID, err)
		return result
	}

	result.Started = true
	spanCtx, span := parent.Tracer().Start(blockCtx, fmt.Sprintf("parallel branch %s", branch.ID),
		trace.WithAttributes(
			attribute.String("parallel.block.id", blockID),
			attribute.String("branch", branch.ID),
			attribute.String("step.id", branch.ID),
		),
	)
	defer span.End()

	branchExec := parent.
		WithIsolatedState(cloneRunStateSnapshot(parentSnapshot)).
		WithContext(spanCtx).
		WithActiveStep(branch.ID)

	if skip, err := e.evaluateCondition(branchExec, branch); err != nil {
		result.Err = toFlowError(err, branch.ID, 0)
		recordBranchError(span, result.Err)
		return result
	} else if skip {
		result.Skipped = true
		if branch.Async {
			parent.AsyncTasks().Register(branch.ID, NewCompletedAsyncTask(branch.ID, nil, nil))
		}
		return result
	}

	if branch.Async {
		runCtx.ensureAsyncWatcher(e, parent)
		result.Err = e.spawnAsyncStep(branchExec, branch, runCtx.flowFinalized)
		if result.Err == nil {
			if task, ok := parent.AsyncTasks().Get(branch.ID); ok {
				asyncTasks.Register(task)
			}
		}
		recordBranchError(span, result.Err)
		return result
	}

	asyncRuntime := asyncRuntimeForExecution(parent)
	if asyncRuntime != nil {
		if err := asyncRuntime.Acquire(branchExec, parent.Metrics(), execFlowID(parent), branch.ID); err != nil {
			result.Err = e.flowError(parent, branch.ID, err)
			recordBranchError(span, result.Err)
			return result
		}
		defer asyncRuntime.Release()
	}

	output, fe, _ := e.runStepPipeline(branchExec, branch)
	if fe != nil {
		result.Err = fe
		recordBranchError(span, result.Err)
		return result
	}
	if output.Response != nil {
		result.Err = &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeRuntimeError),
			Message: "parallel branch cannot produce a response",
			Step:    branch.ID,
		}
		recordBranchError(span, result.Err)
		return result
	}
	if output.Next != "" {
		result.Err = &FlowError{
			Type:    ErrorTypePermanent,
			Code:    string(ErrorCodeRuntimeError),
			Message: "parallel branch cannot set __next",
			Step:    branch.ID,
		}
		recordBranchError(span, result.Err)
		return result
	}
	result.Output = output
	return result
}

func recordBranchError(span trace.Span, fe *FlowError) {
	if fe == nil {
		return
	}
	span.RecordError(fe)
	span.SetStatus(codes.Error, fe.Message)
}

type parallelAsyncTaskTracker struct {
	mu       sync.Mutex
	canceled bool
	tasks    []*AsyncTask
}

func (t *parallelAsyncTaskTracker) Register(task *AsyncTask) {
	if t == nil || task == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.canceled {
		task.Cancel()
		return
	}
	t.tasks = append(t.tasks, task)
}

func (t *parallelAsyncTaskTracker) CancelAll() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.canceled = true
	for _, task := range t.tasks {
		task.Cancel()
	}
}

func (t *parallelAsyncTaskTracker) WaitAll() {
	if t == nil {
		return
	}
	t.mu.Lock()
	tasks := append([]*AsyncTask(nil), t.tasks...)
	t.mu.Unlock()

	for _, task := range tasks {
		if task == nil {
			continue
		}
		<-task.done
	}
}
