package runtime

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestTracingResource_PreservesDefaultMetadata(t *testing.T) {
	res, err := otelResource(map[string]string{
		"deployment.environment": "test",
	})
	if err != nil {
		t.Fatalf("expected otelResource to succeed, got %v", err)
	}
	if res == nil {
		t.Fatal("expected otelResource to return a resource")
	}

	if value, ok := resourceAttribute(res.Attributes(), attribute.Key("deployment.environment")); !ok || value.AsString() != "test" {
		t.Fatalf("expected deployment.environment=test, got %v (present=%v)", value, ok)
	}
	if _, ok := resourceAttribute(res.Attributes(), attribute.Key("service.name")); !ok {
		t.Fatalf("expected tracing resource to preserve default service.name, got %v", res.Attributes())
	}
}

func resourceAttribute(attrs []attribute.KeyValue, key attribute.Key) (attribute.Value, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}
