package runtime

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
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

func TestLoggerForPlugin_SetsSourceOnly(t *testing.T) {
	var buf bytes.Buffer
	base := NewLogger(NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	}))

	base.ForPlugin("stripe").Info("charged")

	output := buf.String()
	if !strings.Contains(output, `"source":"plugin"`) {
		t.Fatalf("expected source=plugin in output, got %s", output)
	}
}

func TestExecutionPluginLogs_DoNotDuplicatePluginAttr(t *testing.T) {
	var buf bytes.Buffer
	container := NewContainer(NewLogger(NewObservabilityLoggerWithWriter(&buf, ObservabilityConfig{
		Logging: LoggingConfig{Level: "debug"},
	})))
	exec := NewExecution(&Flow{ID: "payments"}, container, nil, &testValueStore{values: make(map[string]any)})

	exec.WithActivePlugin("stripe", func() {
		exec.Logger().Info("charged")
	})

	output := buf.String()
	if count := strings.Count(output, `"plugin":"stripe"`); count != 1 {
		t.Fatalf("expected plugin attr once, got count=%d output=%s", count, output)
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

func TestApplyObservabilityDefaults_DefaultsLogExportModeToStdout(t *testing.T) {
	cfg := ObservabilityConfig{}
	if err := ApplyObservabilityDefaults(&cfg); err != nil {
		t.Fatalf("ApplyObservabilityDefaults failed: %v", err)
	}
	if got, want := []string(cfg.Logging.Export.Mode), []string{logExportModeStdout}; !slices.Equal(got, want) {
		t.Fatalf("expected default log export mode %v, got %v", want, got)
	}
}

func TestApplyObservabilityDefaults_DefaultsTracingSampleRateToOneWhenUnset(t *testing.T) {
	cfg := ObservabilityConfig{}
	if err := ApplyObservabilityDefaults(&cfg); err != nil {
		t.Fatalf("ApplyObservabilityDefaults failed: %v", err)
	}
	if cfg.Tracing.SampleRate != 1.0 {
		t.Fatalf("expected default trace sample rate 1.0, got %v", cfg.Tracing.SampleRate)
	}
}

func TestApplyObservabilityDefaults_PreservesExplicitZeroTracingSampleRate(t *testing.T) {
	var cfg ObservabilityConfig
	if err := yaml.Unmarshal([]byte("tracing:\n  sampler: trace_id_ratio\n  sample_rate: 0\n"), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if err := ApplyObservabilityDefaults(&cfg); err != nil {
		t.Fatalf("ApplyObservabilityDefaults failed: %v", err)
	}
	if cfg.Tracing.SampleRate != 0 {
		t.Fatalf("expected explicit zero trace sample rate to be preserved, got %v", cfg.Tracing.SampleRate)
	}
}

func TestValidateObservabilityConfig_RequiresLoggingExportEndpointWhenOTLPEnabled(t *testing.T) {
	err := ValidateObservabilityConfig(ObservabilityConfig{
		Logging: LoggingConfig{
			Export: LogExportConfig{
				Enabled: true,
				Mode:    LogExportModes{logExportModeOTLP},
			},
		},
	})
	if err == nil {
		t.Fatal("expected logging export validation error")
	}
	if !strings.Contains(err.Error(), "Endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
}

func TestLogExportModes_UnmarshalScalarAndSequence(t *testing.T) {
	var scalar struct {
		Mode LogExportModes `yaml:"mode"`
	}
	if err := yaml.Unmarshal([]byte("mode: otlp\n"), &scalar); err != nil {
		t.Fatalf("yaml.Unmarshal scalar failed: %v", err)
	}
	if got, want := []string(scalar.Mode), []string{"otlp"}; !slices.Equal(got, want) {
		t.Fatalf("expected scalar mode %v, got %v", want, got)
	}

	var sequence struct {
		Mode LogExportModes `yaml:"mode"`
	}
	if err := yaml.Unmarshal([]byte("mode: [stdout, otlp]\n"), &sequence); err != nil {
		t.Fatalf("yaml.Unmarshal sequence failed: %v", err)
	}
	if got, want := []string(sequence.Mode), []string{"stdout", "otlp"}; !slices.Equal(got, want) {
		t.Fatalf("expected sequence mode %v, got %v", want, got)
	}
}

func TestInitObservabilityLoggerWithWriter_FansOutToStdoutAndOTLP(t *testing.T) {
	var stdout bytes.Buffer
	var otlp bytes.Buffer

	prev := newOTLPLogHandler
	t.Cleanup(func() { newOTLPLogHandler = prev })
	newOTLPLogHandler = func(cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
		return slog.NewJSONHandler(&otlp, &slog.HandlerOptions{Level: slog.LevelDebug}), func(context.Context) error { return nil }, nil
	}

	logger, shutdown, err := InitObservabilityLoggerWithWriter(&stdout, ObservabilityConfig{
		Logging: LoggingConfig{
			Level: "debug",
			Masking: MaskingConfig{
				Fields:      []string{"password"},
				Placeholder: "***",
			},
			Export: LogExportConfig{
				Enabled:  true,
				Mode:     LogExportModes{logExportModeStdout, logExportModeOTLP},
				Endpoint: "localhost:4317",
			},
		},
	})
	if err != nil {
		t.Fatalf("InitObservabilityLoggerWithWriter failed: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	}()

	logger.Info("fanout", "password", "secret")

	for _, output := range []string{stdout.String(), otlp.String()} {
		if !strings.Contains(output, `"password":"***"`) {
			t.Fatalf("expected masked password in output, got %s", output)
		}
		if !strings.Contains(output, `"source":"framework"`) {
			t.Fatalf("expected source in output, got %s", output)
		}
	}
}

func TestInitObservabilityLoggerWithWriter_OTLPOnlySkipsStdout(t *testing.T) {
	var stdout bytes.Buffer
	var otlp bytes.Buffer

	prev := newOTLPLogHandler
	t.Cleanup(func() { newOTLPLogHandler = prev })
	newOTLPLogHandler = func(cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
		return slog.NewJSONHandler(&otlp, &slog.HandlerOptions{Level: slog.LevelDebug}), func(context.Context) error { return nil }, nil
	}

	logger, shutdown, err := InitObservabilityLoggerWithWriter(&stdout, ObservabilityConfig{
		Logging: LoggingConfig{
			Level: "debug",
			Export: LogExportConfig{
				Enabled:  true,
				Mode:     LogExportModes{logExportModeOTLP},
				Endpoint: "localhost:4317",
			},
		},
	})
	if err != nil {
		t.Fatalf("InitObservabilityLoggerWithWriter failed: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}
	}()

	logger.Info("otlp only")

	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output, got %s", stdout.String())
	}
	if !strings.Contains(otlp.String(), "otlp only") {
		t.Fatalf("expected otlp output, got %s", otlp.String())
	}
}

func TestJoinShutdowns_JoinsErrors(t *testing.T) {
	wantErr := errors.New("boom")
	err := joinShutdowns([]func(context.Context) error{
		func(context.Context) error { return wantErr },
		func(context.Context) error { return nil },
	})(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected joined error to contain %v, got %v", wantErr, err)
	}
}

func TestInitObservability_CleansUpLoggerWhenTracingInitFails(t *testing.T) {
	prevInitLogger := initObservabilityLogger
	prevInitTracing := initTracingRuntime
	prevInitMetrics := initMetricsRuntime
	t.Cleanup(func() {
		initObservabilityLogger = prevInitLogger
		initTracingRuntime = prevInitTracing
		initMetricsRuntime = prevInitMetrics
	})

	loggerShutdownCalled := false
	initObservabilityLogger = func(cfg ObservabilityConfig) (*slog.Logger, func(context.Context) error, error) {
		return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(context.Context) error {
			loggerShutdownCalled = true
			return nil
		}, nil
	}
	initTracingRuntime = func(cfg TracingConfig) (trace.Tracer, func(context.Context) error, error) {
		return nil, nil, errors.New("tracing boom")
	}
	initMetricsRuntime = func(cfg MetricsConfig) (*Metrics, func(context.Context) error, error) {
		t.Fatal("metrics init should not be called when tracing init fails")
		return nil, nil, nil
	}

	_, err := InitObservability(ObservabilityConfig{})
	if err == nil {
		t.Fatal("expected InitObservability to fail")
	}
	if !loggerShutdownCalled {
		t.Fatal("expected logger shutdown cleanup when tracing init fails")
	}
}
