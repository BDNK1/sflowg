package runtime

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type IterationSource interface {
	Len() int
	At(i int) any
}

type reflectIterationSource struct {
	value reflect.Value
}

func (s reflectIterationSource) Len() int {
	return s.value.Len()
}

func (s reflectIterationSource) At(i int) any {
	return s.value.Index(i).Interface()
}

type batchIterationSource struct {
	value     reflect.Value
	batchSize int
}

func (s batchIterationSource) Len() int {
	if s.value.Len() == 0 {
		return 0
	}
	return (s.value.Len() + s.batchSize - 1) / s.batchSize
}

func (s batchIterationSource) At(i int) any {
	start := i * s.batchSize
	end := start + s.batchSize
	if end > s.value.Len() {
		end = s.value.Len()
	}
	if s.value.Kind() == reflect.Slice {
		return s.value.Slice(start, end).Interface()
	}
	items := make([]any, end-start)
	for j := start; j < end; j++ {
		items[j-start] = s.value.Index(j).Interface()
	}
	return items
}

type foreachIterationResult struct {
	Index  int
	Values map[string]any
	Err    *FlowError
}

type foreachIterationJob struct {
	Index int
	Item  any
}

func newIterationSource(value any, batchSize int) (IterationSource, *FlowError) {
	if value == nil {
		return nil, iterationSourceError("foreach source evaluated to nil; expected array or slice")
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return nil, iterationSourceError("foreach source evaluated to nil slice; expected array or slice")
		}
	case reflect.Array:
	default:
		return nil, iterationSourceError(fmt.Sprintf("foreach source evaluated to %T; expected array or slice", value))
	}
	if batchSize > 0 {
		return batchIterationSource{value: v, batchSize: batchSize}, nil
	}
	return reflectIterationSource{value: v}, nil
}

func iterationSourceError(message string) *FlowError {
	return &FlowError{
		Type:    ErrorTypePermanent,
		Code:    string(ErrorCodeRuntimeError),
		Message: message,
	}
}

func (e *Executor) executeForeachBlock(
	execution *Execution,
	node FlowNode,
	block ForeachBlock,
	runCtx *executionRunContext,
) (flowErr *FlowError) {
	options := effectiveForeachOptions(execution, block.Options)
	parentCtx := execution.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	spanCtx, span := execution.Tracer().Start(parentCtx, fmt.Sprintf("foreach %s", node.ID),
		trace.WithAttributes(foreachSpanAttributes(node.ID, -1, block.Parallel, options)...),
	)
	defer func() {
		recordForeachSpanError(span, flowErr)
		span.End()
	}()
	execution = execution.WithContext(spanCtx)

	sourceValue, err := e.evaluator.EvalExpression(
		execution.WithActiveStep(node.ID),
		block.Expr,
		block.ExprProgram,
		block.ExprStoreKeys,
		nil,
	)
	if err != nil {
		e.commitEmptyCollects(execution, block)
		return foreachSourceError(err, node.ID)
	}

	source, fe := newIterationSource(sourceValue, block.BatchSize)
	if fe != nil {
		e.commitEmptyCollects(execution, block)
		fe.Step = node.ID
		return stampForeachNodeError(fe, node.ID)
	}

	collects := allocateForeachCollects(block, source.Len())
	parentView := NewReadOnlyValueView(execution.State().Store(), block.ParentStoreKeys)
	if block.Parallel {
		return e.executeParallelForeachBlock(execution, node, block, options, source, collects, parentView, runCtx)
	}

	for i := 0; i < source.Len(); i++ {
		result := e.runForeachIteration(execution, node.ID, block, options, parentView, source.At(i), i, runCtx)
		if result.Err != nil {
			e.commitForeachCollects(execution, collects)
			return stampForeachError(result.Err, node.ID, i)
		}
		for alias, value := range result.Values {
			collects[alias][i] = value
		}
	}

	e.commitForeachCollects(execution, collects)
	return nil
}

func (e *Executor) executeParallelForeachBlock(
	execution *Execution,
	node FlowNode,
	block ForeachBlock,
	options ParallelOptions,
	source IterationSource,
	collects map[string][]any,
	parentView ReadOnlyValueView,
	runCtx *executionRunContext,
) *FlowError {
	if source.Len() == 0 {
		e.commitForeachCollects(execution, collects)
		return nil
	}

	parentCtx := execution.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	foreachCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	results := make([]foreachIterationResult, source.Len())
	jobs := make(chan foreachIterationJob)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr *FlowError
	activeScopes := newForeachAsyncScopeTracker()
	asyncRuntime := asyncRuntimeForExecution(execution)

	for worker := 0; worker < options.MaxInFlight; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if foreachCtx.Err() != nil {
					return
				}
				select {
				case <-foreachCtx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					if foreachCtx.Err() != nil {
						return
					}
					if asyncRuntime != nil {
						if err := asyncRuntime.Acquire(foreachCtx, execution.Metrics(), execFlowID(execution), node.ID); err != nil {
							result := foreachIterationResult{Index: job.Index, Err: e.flowError(execution, node.ID, err)}
							results[job.Index] = result
							if options.OnFailure == OnFailureFailFast && foreachCtx.Err() == nil {
								mu.Lock()
								if firstErr == nil {
									firstErr = result.Err
									cancel()
									activeScopes.CancelAllLocal()
								}
								mu.Unlock()
							}
							continue
						}
					}
					result, asyncScope := e.runParallelForeachIteration(execution, node.ID, block, options, parentView, job.Item, job.Index, foreachCtx, activeScopes, runCtx)
					if asyncRuntime != nil {
						asyncRuntime.Release()
					}
					if result.Err != nil && options.OnFailure == OnFailureFailFast && foreachCtx.Err() == nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = result.Err
							cancel()
							activeScopes.CancelAllLocal()
						}
						mu.Unlock()
						asyncScope.CancelLocal()
						_ = asyncScope.WaitLocal(context.Background())
					} else {
						if foreachCtx.Err() != nil {
							asyncScope.CancelLocal()
							_ = asyncScope.WaitLocal(context.Background())
						} else {
							asyncScope.DetachLocal(runCtx.detachedAsync)
						}
					}
					results[job.Index] = result
				}
			}
		}()
	}

	for i := 0; i < source.Len(); i++ {
		select {
		case <-foreachCtx.Done():
			i = source.Len()
		case jobs <- foreachIterationJob{Index: i, Item: source.At(i)}:
		}
	}
	close(jobs)
	wg.Wait()

	if options.OnFailure == OnFailureFailFast {
		if firstErr != nil {
			activeScopes.CancelAllLocal()
			_ = activeScopes.WaitAllLocal(context.Background())
			e.copyForeachCollectResults(collects, results)
			e.commitForeachCollects(execution, collects)
			return stampForeachError(firstErr, node.ID, firstFailureIteration(results, firstErr))
		}
		if err := foreachCtx.Err(); err != nil {
			activeScopes.CancelAllLocal()
			_ = activeScopes.WaitAllLocal(context.Background())
			e.copyForeachCollectResults(collects, results)
			e.commitForeachCollects(execution, collects)
			return stampForeachNodeError(e.flowError(execution, node.ID, err), node.ID)
		}
		e.copyForeachCollectResults(collects, results)
		e.commitForeachCollects(execution, collects)
		return nil
	}

	if err := foreachCtx.Err(); err != nil {
		activeScopes.CancelAllLocal()
		_ = activeScopes.WaitAllLocal(context.Background())
		e.copyForeachCollectResults(collects, results)
		e.commitForeachCollects(execution, collects)
		return stampForeachNodeError(e.flowError(execution, node.ID, err), node.ID)
	}
	e.copyForeachCollectResults(collects, results)
	e.commitForeachCollects(execution, collects)
	return foreachWaitAllFailure(node.ID, results)
}

func effectiveForeachOptions(execution *Execution, options ParallelOptions) ParallelOptions {
	cfg := NormalizeParallelConfig(ParallelConfig{})
	if execution != nil && execution.Container != nil {
		cfg = execution.Container.RuntimeConfig().Parallel
	}
	if options.MaxInFlight <= 0 {
		options.MaxInFlight = cfg.ForeachDefaultMaxInFlight
	}
	if options.MaxInFlight <= 0 {
		options.MaxInFlight = DefaultParallelForeachMaxInFlight
	}
	if options.OnFailure == "" {
		options.OnFailure = cfg.DefaultOnFailure
	}
	if options.OnFailure == "" {
		options.OnFailure = OnFailureWaitAll
	}
	return options
}

func (e *Executor) runParallelForeachIteration(
	parent *Execution,
	foreachID string,
	block ForeachBlock,
	options ParallelOptions,
	parentView ReadOnlyValueView,
	item any,
	iteration int,
	ctx context.Context,
	activeScopes *foreachAsyncScopeTracker,
	runCtx *executionRunContext,
) (foreachIterationResult, *AsyncScope) {
	iterationExec, asyncScope := newForeachIterationExecution(parent, block, parentView, item, ctx)
	activeScopes.Register(iteration, asyncScope)
	defer activeScopes.Unregister(iteration)
	return e.runForeachIterationExecution(iterationExec, foreachID, block, options, iteration, runCtx), asyncScope
}

func (e *Executor) runForeachIteration(
	parent *Execution,
	foreachID string,
	block ForeachBlock,
	options ParallelOptions,
	parentView ReadOnlyValueView,
	item any,
	iteration int,
	runCtx *executionRunContext,
) foreachIterationResult {
	iterationExec, asyncScope := newForeachIterationExecution(parent, block, parentView, item, nil)
	defer asyncScope.DetachLocal(runCtx.detachedAsync)
	return e.runForeachIterationExecution(iterationExec, foreachID, block, options, iteration, runCtx)
}

func newForeachIterationExecution(
	parent *Execution,
	block ForeachBlock,
	parentView ReadOnlyValueView,
	item any,
	ctx context.Context,
) (*Execution, *AsyncScope) {
	localStore := NewScopedValueStore(parentView)
	localStore.SetNested(block.ItemVar, item)
	asyncScope := NewAsyncScope(parent.AsyncTasks())
	iterationExec := parent.
		WithValueStore(localStore).
		WithAsyncScope(asyncScope)
	if ctx != nil {
		iterationExec = iterationExec.WithContext(ctx)
	}
	return iterationExec, asyncScope
}

func (e *Executor) runForeachIterationExecution(
	iterationExec *Execution,
	foreachID string,
	block ForeachBlock,
	options ParallelOptions,
	iteration int,
	runCtx *executionRunContext,
) (result foreachIterationResult) {
	parentCtx := iterationExec.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	spanCtx, span := iterationExec.Tracer().Start(parentCtx, fmt.Sprintf("foreach iteration %d", iteration),
		trace.WithAttributes(foreachSpanAttributes(foreachID, iteration, block.Parallel, options)...),
	)
	defer func() {
		recordForeachSpanError(span, result.Err)
		span.End()
	}()
	iterationExec = iterationExec.WithContext(spanCtx)

	for _, step := range block.Steps {
		if step.CompensateBody != "" {
			return foreachIterationResult{Index: iteration, Err: &FlowError{
				Type:    ErrorTypePermanent,
				Code:    string(ErrorCodeRuntimeError),
				Message: "foreach body step cannot have a compensate block",
				Step:    step.ID,
			}}
		}
		result, fe := e.executeStepNode(iterationExec, step, runCtx)
		if fe != nil {
			return foreachIterationResult{Index: iteration, Err: fe}
		}
		if result.Next != "" {
			return foreachIterationResult{Index: iteration, Err: &FlowError{
				Type:    ErrorTypePermanent,
				Code:    string(ErrorCodeRuntimeError),
				Message: "foreach body step cannot set __next",
				Step:    step.ID,
			}}
		}
		if iterationExec.State().Response() != nil {
			return foreachIterationResult{Index: iteration, Err: &FlowError{
				Type:    ErrorTypePermanent,
				Code:    string(ErrorCodeRuntimeError),
				Message: "foreach body step cannot produce a response",
				Step:    step.ID,
			}}
		}
	}

	values := make(map[string]any, len(block.Collects))
	for _, collect := range block.Collects {
		value, err := e.evaluator.EvalExpression(
			iterationExec.WithActiveStep(collect.Alias),
			collect.Expr,
			collect.ExprProgram,
			collect.StoreKeys,
			nil,
		)
		if err != nil {
			return foreachIterationResult{Index: iteration, Err: foreachCollectError(err, foreachID, collect.Alias, iteration)}
		}
		values[collect.Alias] = value
	}
	return foreachIterationResult{Index: iteration, Values: values}
}

func foreachSpanAttributes(foreachID string, iteration int, parallel bool, options ParallelOptions) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("foreach.id", foreachID),
		attribute.Bool("foreach.parallel", parallel),
		attribute.Int("parallel.max_in_flight", options.MaxInFlight),
		attribute.String("parallel.on_failure", string(options.OnFailure)),
	}
	if iteration >= 0 {
		attrs = append(attrs, attribute.Int("iteration", iteration))
	}
	return attrs
}

func recordForeachSpanError(span trace.Span, fe *FlowError) {
	if fe == nil {
		return
	}
	span.RecordError(fe)
	span.SetStatus(codes.Error, fe.Message)
}

func (e *Executor) copyForeachCollectResults(collects map[string][]any, results []foreachIterationResult) {
	for _, result := range results {
		if result.Err != nil || result.Values == nil {
			continue
		}
		for alias, value := range result.Values {
			if values, ok := collects[alias]; ok && result.Index >= 0 && result.Index < len(values) {
				values[result.Index] = value
			}
		}
	}
}

func foreachWaitAllFailure(foreachID string, results []foreachIterationResult) *FlowError {
	failures := make([]FlowFailure, 0)
	var single *FlowError
	for i, result := range results {
		if result.Err == nil {
			continue
		}
		stamped := stampForeachError(result.Err, foreachID, i)
		single = stamped
		iteration := i
		failures = append(failures, FlowFailure{
			Iteration: &iteration,
			Step:      stamped.Step,
			Error:     FlowErrorDetailFrom(stamped),
		})
	}
	if len(failures) == 0 {
		return nil
	}
	if len(failures) == 1 {
		return single
	}
	return NewGroupedFailure(foreachID, "multiple foreach iterations failed", failures)
}

func firstFailureIteration(results []foreachIterationResult, target *FlowError) int {
	for i, result := range results {
		if result.Err == target {
			return i
		}
	}
	if target != nil && target.Meta != nil {
		if iteration, ok := target.Meta["iteration"].(int); ok {
			return iteration
		}
	}
	return 0
}

type foreachAsyncScopeTracker struct {
	mu     sync.Mutex
	scopes map[int]*AsyncScope
}

func newForeachAsyncScopeTracker() *foreachAsyncScopeTracker {
	return &foreachAsyncScopeTracker{scopes: make(map[int]*AsyncScope)}
}

func (t *foreachAsyncScopeTracker) Register(iteration int, scope *AsyncScope) {
	if t == nil || scope == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scopes[iteration] = scope
}

func (t *foreachAsyncScopeTracker) Unregister(iteration int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.scopes, iteration)
}

func (t *foreachAsyncScopeTracker) CancelAllLocal() {
	for _, scope := range t.snapshot() {
		scope.CancelLocal()
	}
}

func (t *foreachAsyncScopeTracker) WaitAllLocal(ctx context.Context) error {
	for _, scope := range t.snapshot() {
		if err := scope.WaitLocal(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (t *foreachAsyncScopeTracker) snapshot() []*AsyncScope {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*AsyncScope, 0, len(t.scopes))
	for _, scope := range t.scopes {
		out = append(out, scope)
	}
	return out
}

func allocateForeachCollects(block ForeachBlock, length int) map[string][]any {
	collects := make(map[string][]any, len(block.Collects))
	for _, collect := range block.Collects {
		collects[collect.Alias] = make([]any, length)
	}
	return collects
}

func (e *Executor) commitEmptyCollects(execution *Execution, block ForeachBlock) {
	collects := allocateForeachCollects(block, 0)
	e.commitForeachCollects(execution, collects)
}

func (e *Executor) commitForeachCollects(execution *Execution, collects map[string][]any) {
	for alias, values := range collects {
		execution.State().Store().SetNested(alias, values)
	}
}

func foreachSourceError(err error, foreachID string) *FlowError {
	fe := toFlowError(err, foreachID, 0)
	fe.Step = foreachID
	return stampForeachNodeError(fe, foreachID)
}

func foreachCollectError(err error, foreachID string, alias string, iteration int) *FlowError {
	fe := toFlowError(err, alias, 0)
	fe.Step = alias
	fe = stampForeachError(fe, foreachID, iteration)
	if fe.Meta == nil {
		fe.Meta = make(map[string]any, 3)
	}
	fe.Meta["collect"] = alias
	return fe
}

func stampForeachNodeError(fe *FlowError, foreachID string) *FlowError {
	if fe == nil {
		return nil
	}
	clone := *fe
	clone.Meta = cloneMeta(fe.Meta)
	if clone.Meta == nil {
		clone.Meta = make(map[string]any, 1)
	}
	clone.Meta["foreach"] = foreachID
	return &clone
}
