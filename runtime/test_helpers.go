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

func (s *flatValueStore) All() map[string]any {
	return s.values
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
