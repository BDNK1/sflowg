package runtime

import (
	"context"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const tracerName = "github.com/BDNK1/sflowg/runtime"

func newNoopTracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer(tracerName)
}

// InitTracing creates the OTel TracerProvider from config.
// Returns a Tracer and a shutdown function. When tracing is disabled,
// returns a noop tracer and a no-op shutdown.
func InitTracing(cfg TracingConfig) (trace.Tracer, func(context.Context) error, error) {
	if !cfg.Enabled {
		return newNoopTracer(), func(context.Context) error { return nil }, nil
	}

	exporterOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(context.Background(), exporterOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(tracingSampler(cfg)),
	}
	if res, err := tracingResource(cfg.Attributes); err != nil {
		return nil, nil, fmt.Errorf("build tracing resource: %w", err)
	} else if res != nil {
		options = append(options, sdktrace.WithResource(res))
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(provider)

	return provider.Tracer(tracerName), provider.Shutdown, nil
}

func tracingSampler(cfg TracingConfig) sdktrace.Sampler {
	switch cfg.Sampler {
	case "", "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "trace_id_ratio":
		return sdktrace.TraceIDRatioBased(cfg.SampleRate)
	case "parent_based":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))
	default:
		return sdktrace.AlwaysSample()
	}
}

func tracingAttributes(values map[string]string) []attribute.KeyValue {
	if len(values) == 0 {
		return nil
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	attrs := make([]attribute.KeyValue, 0, len(keys))
	for _, key := range keys {
		attrs = append(attrs, attribute.String(key, values[key]))
	}
	return attrs
}

func tracingResource(values map[string]string) (*resource.Resource, error) {
	attrs := tracingAttributes(values)
	if len(attrs) == 0 {
		return nil, nil
	}
	return resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
}
