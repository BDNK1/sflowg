package runtime

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// MetricType identifies the kind of metric instrument.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeGauge     MetricType = "gauge"
)

// MetricConfig declares a single metric instrument in flow-config.yaml.
//
//	observability:
//	  metrics:
//	    - name: checkout_attempts
//	      type: counter
//	      description: "Number of checkout attempts"
//	    - name: checkout_amounts
//	      type: histogram
//	      unit: "USD"
type MetricConfig struct {
	Name        string     `yaml:"name"        validate:"required"`
	Type        MetricType `yaml:"type"        validate:"required,oneof=counter histogram gauge"`
	Description string     `yaml:"description" validate:"omitempty"`
	Unit        string     `yaml:"unit"        validate:"omitempty"`
}

// MetricHandle is the DSL-facing handle for a single metric instrument.
// Only the methods relevant to each MetricType are called by the DSL bridge;
// the others are no-ops on the concrete types.
type MetricHandle interface {
	Type() MetricType
	// Add increments a counter or adjusts an up-down counter.
	Add(ctx context.Context, delta float64) error
	// Record observes a value on a histogram.
	Record(ctx context.Context, value float64) error
	// Set records the current absolute value of a gauge.
	Set(ctx context.Context, value float64) error
}

// InitMetrics creates one MetricHandle per MetricConfig using the given meter.
// Returns a map keyed by metric name ready to be stored on Container.Metrics.
func InitMetrics(cfgs []MetricConfig, meter metric.Meter) (map[string]MetricHandle, error) {
	handles := make(map[string]MetricHandle, len(cfgs))
	for _, cfg := range cfgs {
		h, err := newMetricHandle(cfg, meter)
		if err != nil {
			return nil, fmt.Errorf("init metric %q: %w", cfg.Name, err)
		}
		handles[cfg.Name] = h
	}
	return handles, nil
}

func newMetricHandle(cfg MetricConfig, meter metric.Meter) (MetricHandle, error) {
	switch cfg.Type {
	case MetricTypeCounter:
		opts := []metric.Float64CounterOption{metric.WithDescription(cfg.Description)}
		if cfg.Unit != "" {
			opts = append(opts, metric.WithUnit(cfg.Unit))
		}
		c, err := meter.Float64Counter(cfg.Name, opts...)
		if err != nil {
			return nil, err
		}
		return &otelCounterHandle{counter: c}, nil

	case MetricTypeHistogram:
		opts := []metric.Float64HistogramOption{metric.WithDescription(cfg.Description)}
		if cfg.Unit != "" {
			opts = append(opts, metric.WithUnit(cfg.Unit))
		}
		h, err := meter.Float64Histogram(cfg.Name, opts...)
		if err != nil {
			return nil, err
		}
		return &otelHistogramHandle{histogram: h}, nil

	case MetricTypeGauge:
		opts := []metric.Float64GaugeOption{metric.WithDescription(cfg.Description)}
		if cfg.Unit != "" {
			opts = append(opts, metric.WithUnit(cfg.Unit))
		}
		g, err := meter.Float64Gauge(cfg.Name, opts...)
		if err != nil {
			return nil, err
		}
		return &otelGaugeHandle{gauge: g}, nil

	default:
		return nil, fmt.Errorf("unknown metric type %q", cfg.Type)
	}
}

// otelCounterHandle wraps a Float64Counter.
// DSL methods: inc(), add(n)
type otelCounterHandle struct {
	counter metric.Float64Counter
}

func (h *otelCounterHandle) Type() MetricType { return MetricTypeCounter }
func (h *otelCounterHandle) Add(ctx context.Context, delta float64) error {
	h.counter.Add(ctx, delta)
	return nil
}
func (h *otelCounterHandle) Record(_ context.Context, _ float64) error { return nil }
func (h *otelCounterHandle) Set(_ context.Context, _ float64) error    { return nil }

// otelHistogramHandle wraps a Float64Histogram.
// DSL methods: record(v)
type otelHistogramHandle struct {
	histogram metric.Float64Histogram
}

func (h *otelHistogramHandle) Type() MetricType { return MetricTypeHistogram }
func (h *otelHistogramHandle) Add(_ context.Context, _ float64) error { return nil }
func (h *otelHistogramHandle) Record(ctx context.Context, value float64) error {
	h.histogram.Record(ctx, value)
	return nil
}
func (h *otelHistogramHandle) Set(_ context.Context, _ float64) error { return nil }

// otelGaugeHandle wraps a Float64Gauge.
// DSL methods: set(v)
type otelGaugeHandle struct {
	gauge metric.Float64Gauge
}

func (h *otelGaugeHandle) Type() MetricType { return MetricTypeGauge }
func (h *otelGaugeHandle) Add(_ context.Context, _ float64) error    { return nil }
func (h *otelGaugeHandle) Record(_ context.Context, _ float64) error { return nil }
func (h *otelGaugeHandle) Set(ctx context.Context, value float64) error {
	h.gauge.Record(ctx, value)
	return nil
}
