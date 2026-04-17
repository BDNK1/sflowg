package runtime

import (
	"context"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// NewFlatValueStore returns a simple ValueStore for use in tests.
// It's a flat map-based store suitable for non-hierarchical test scenarios.
func NewFlatValueStore() ValueStore {
	return &flatValueStore{values: make(map[string]any)}
}

type flatValueStore struct {
	values map[string]any
}

func (s *flatValueStore) Set(key string, value any) {
	s.values[key] = value
}

func (s *flatValueStore) Get(key string) (any, bool) {
	v, ok := s.values[key]
	return v, ok
}

func (s *flatValueStore) SetNested(prefix string, value any) {
	s.values[prefix] = value
}

func (s *flatValueStore) Snapshot() map[string]any {
	out := make(map[string]any, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

// NewTestMetricsWithReader creates a Metrics instance backed by a ManualReader
// for use in external test packages. It initializes predeclared user metrics
// from the provided declarations.
func NewTestMetricsWithReader(reader *sdkmetric.ManualReader, decls map[string]UserMetricDecl) (*Metrics, error) {
	cfg := MetricsConfig{
		User: UserMetricsConfig{Declarations: decls},
	}
	provider, err := newMeterProvider(cfg, reader)
	if err != nil {
		return nil, err
	}
	// Note: caller is responsible for provider shutdown via reader.

	metrics, err := newMetrics(provider)
	if err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, err
	}

	if err := metrics.InitUserMetrics(decls); err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, err
	}

	return metrics, nil
}

type isolatedTestStepRunner struct {
	executor StepExecutor
}

func newIsolatedTestStepRunner(executor StepExecutor) StepRunner {
	return isolatedTestStepRunner{executor: executor}
}

func (r isolatedTestStepRunner) RunStep(ctx context.Context, execution *Execution, input StepInput) (StepOutput, error) {
	store := NewValueStore()
	for k, v := range input.Input {
		store.SetNested(k, v)
	}

	isolated := execution.WithIsolatedState(NewRunState(store))
	step := Step{
		ID:       input.StepID,
		Body:     input.Body,
		Timeout:  input.Timeout,
		Compiled: input.Compiled,
	}

	next, err := r.executor.ExecuteStep(ctx, isolated, step)
	if err != nil {
		return StepOutput{}, err
	}

	result, _ := isolated.State().Store().Get(input.StepID)
	return StepOutput{
		Result:   result,
		Response: isolated.State().Response(),
		Next:     next,
	}, nil
}
