package runtime

import (
	"context"
	"time"
)

const DefaultAsyncRuntimeMaxInFlight = 256

type AsyncConfig struct {
	RuntimeMaxInFlight int
}

type AsyncRuntime struct {
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
}

func NewAsyncRuntime(cfg AsyncConfig) *AsyncRuntime {
	max := cfg.RuntimeMaxInFlight
	if max <= 0 {
		max = DefaultAsyncRuntimeMaxInFlight
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncRuntime{
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, max),
	}
}

func (r *AsyncRuntime) Context() context.Context {
	if r == nil || r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

func (r *AsyncRuntime) Acquire(ctx context.Context, metrics *Metrics, flowID string, stepID string) error {
	if r == nil {
		return nil
	}

	select {
	case r.sem <- struct{}{}:
		return nil
	default:
	}

	start := time.Now()
	select {
	case r.sem <- struct{}{}:
		metrics.RecordRuntimeBudgetSaturation(ctx, flowID, stepID, time.Since(start))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

func (r *AsyncRuntime) Release() {
	if r == nil {
		return
	}
	<-r.sem
}

func (r *AsyncRuntime) Shutdown() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
}
