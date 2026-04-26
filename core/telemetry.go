package runtime

import (
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const instrumentationName = "github.com/BDNK1/sflowg/core"

func newNoopTracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer(instrumentationName)
}

func otelAttributes(values map[string]string) []attribute.KeyValue {
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

func otelResource(values map[string]string) (*resource.Resource, error) {
	attrs := otelAttributes(values)
	if len(attrs) == 0 {
		return nil, nil
	}
	return resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
}
