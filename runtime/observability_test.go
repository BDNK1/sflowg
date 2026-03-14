package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

func TestObservabilityHandler_EnrichesExecutionContext(t *testing.T) {
	var buf bytes.Buffer
	handler := newObservabilityHandler(&buf, LoggingConfig{Level: "debug"}, "framework")
	logger := slog.New(handler)

	exec := &Execution{
		ID:           "exec-123",
		Flow:         &Flow{ID: "payment_flow"},
		activeStepID: "charge_card",
		activePlugin: "http",
	}

	logger.InfoContext(exec, "test log")

	output := buf.String()
	for _, want := range []string{`"execution_id":"exec-123"`, `"flow_id":"payment_flow"`, `"step_id":"charge_card"`, `"plugin":"http"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected log output to contain %s, got %s", want, output)
		}
	}
}

func TestObservabilityHandler_AppliesMaskingAndTruncation(t *testing.T) {
	var buf bytes.Buffer
	handler := newObservabilityHandler(&buf, LoggingConfig{
		Level:           "debug",
		MaxPayloadBytes: 16,
		Masking: MaskingConfig{
			Fields:      []string{"password"},
			Placeholder: "***",
		},
	}, "user")
	logger := slog.New(handler)

	logger.Info("test log",
		"password", "secret-value",
		"body", strings.Repeat("x", 32))

	output := buf.String()
	if !strings.Contains(output, `"password":"***"`) {
		t.Fatalf("expected masked password in log output, got %s", output)
	}
	if !strings.Contains(output, "[truncated]") {
		t.Fatalf("expected truncated marker in log output, got %s", output)
	}
}

func TestObservabilityHandler_AppliesSourceLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := newObservabilityHandler(&buf, LoggingConfig{
		Level: "debug",
		Sources: LogSourcesConfig{
			Plugin: "error",
		},
	}, "framework").(*observabilityHandler)

	pluginLogger := slog.New(baseHandler.withSource("plugin"))

	pluginLogger.Info("plugin info")
	if buf.Len() != 0 {
		t.Fatalf("expected plugin info log to be filtered, got %s", buf.String())
	}

	pluginLogger.Error("plugin error")
	if !strings.Contains(buf.String(), "plugin error") {
		t.Fatalf("expected plugin error log to pass through, got %s", buf.String())
	}
}

func TestObservabilityHandler_PreservesNumericKindsWhenUnchanged(t *testing.T) {
	var buf bytes.Buffer
	handler := newObservabilityHandler(&buf, LoggingConfig{Level: "debug"}, "framework")
	logger := slog.New(handler)

	attr := handler.(*observabilityHandler).sanitizeAttr(slog.Int64("count", 42))
	if attr.Value.Kind() != slog.KindInt64 {
		t.Fatalf("expected slog.KindInt64, got %s", attr.Value.Kind())
	}

	logger.Info("numeric log", "count", int64(42))
	if !strings.Contains(buf.String(), `"count":42`) {
		t.Fatalf("expected numeric JSON output, got %s", buf.String())
	}
}

func TestObservabilityHandler_InjectsSourceFromHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := newObservabilityHandler(&buf, LoggingConfig{Level: "debug"}, "plugin")
	slog.New(handler).Info("msg")

	if !strings.Contains(buf.String(), `"source":"plugin"`) {
		t.Fatalf("expected source=plugin injected by handler, got %s", buf.String())
	}
}

func TestLoggerForPlugin_SetsSourceAndPluginName(t *testing.T) {
	var buf bytes.Buffer
	base := NewLogger(NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	}))

	base.ForPlugin("stripe").Info("charged")

	output := buf.String()
	for _, want := range []string{`"source":"plugin"`, `"plugin":"stripe"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %s in output, got %s", want, output)
		}
	}
}

func TestLoggerForUser_SetsSourceUser(t *testing.T) {
	var buf bytes.Buffer
	base := NewLogger(NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	}))

	base.ForUser().Info("user log")

	if !strings.Contains(buf.String(), `"source":"user"`) {
		t.Fatalf("expected source=user in output, got %s", buf.String())
	}
}

func TestObservabilityHandler_InjectsTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newObservabilityHandler(&buf, LoggingConfig{Level: "debug"}, "framework"))

	traceID := trace.TraceID{0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe, 0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe}
	spanID := trace.SpanID{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	logger.InfoContext(ctx, "traced log")

	output := buf.String()
	for _, want := range []string{
		`"trace_id":"1032547698badcfe1032547698badcfe"`,
		`"span_id":"aabbccddeeff0011"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %s in output, got %s", want, output)
		}
	}
}

func TestValidateObservabilityConfig_RequiresTracingEndpointWhenEnabled(t *testing.T) {
	err := ValidateObservabilityConfig(ObservabilityConfig{
		Tracing: TracingConfig{Enabled: true},
	})
	if err == nil {
		t.Fatal("expected tracing validation error")
	}
	if !strings.Contains(err.Error(), "Endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
}
