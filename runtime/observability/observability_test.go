package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BDNK1/sflowg/runtime"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

type testObservabilityContext struct{}

func (testObservabilityContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (testObservabilityContext) Done() <-chan struct{}       { return nil }
func (testObservabilityContext) Err() error                  { return nil }
func (testObservabilityContext) Value(any) any               { return nil }
func (testObservabilityContext) ObservabilityAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("execution_id", "exec-123"),
		slog.String("flow_id", "payment_flow"),
		slog.String("step_id", "charge_card"),
		slog.String("plugin", "http"),
	}
}

func TestObservabilityHandler_EnrichesExecutionContext(t *testing.T) {
	var buf bytes.Buffer
	handler := newObservabilityHandler(&buf, LoggingConfig{Level: "debug"}, "framework")
	logger := slog.New(handler)

	logger.InfoContext(testObservabilityContext{}, "test log")

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
	base := runtime.NewLogger(NewLoggerWithWriter(&buf, Config{
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
	container := runtime.NewContainer(runtime.NewLogger(NewLoggerWithWriter(&buf, Config{
		Logging: LoggingConfig{Level: "debug"},
	})))
	exec := runtime.NewExecution(&runtime.Flow{ID: "payments"}, container, nil, runtime.NewValueStore())

	pluginExec := exec.WithActivePlugin("stripe")
	pluginExec.Logger().Info("charged")

	output := buf.String()
	if count := strings.Count(output, `"plugin":"stripe"`); count != 1 {
		t.Fatalf("expected plugin attr once, got count=%d output=%s", count, output)
	}
}

func TestLoggerForUser_SetsSourceUser(t *testing.T) {
	var buf bytes.Buffer
	base := runtime.NewLogger(NewLoggerWithWriter(&buf, Config{
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

func TestValidateConfig_RequiresTracingEndpointWhenEnabled(t *testing.T) {
	err := ValidateConfig(Config{
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
	cfg := Config{}
	if err := ApplyDefaults(&cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}
	if got, want := []string(cfg.Logging.Export.Mode), []string{logExportModeStdout}; !slices.Equal(got, want) {
		t.Fatalf("expected default log export mode %v, got %v", want, got)
	}
}

func TestApplyObservabilityDefaults_DefaultsTracingSampleRateToOneWhenUnset(t *testing.T) {
	cfg := Config{}
	if err := ApplyDefaults(&cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}
	if cfg.Tracing.SampleRate != 1.0 {
		t.Fatalf("expected default trace sample rate 1.0, got %v", cfg.Tracing.SampleRate)
	}
}

func TestApplyObservabilityDefaults_PreservesExplicitZeroTracingSampleRate(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("tracing:\n  sampler: trace_id_ratio\n  sample_rate: 0\n"), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if err := ApplyDefaults(&cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}
	if cfg.Tracing.SampleRate != 0 {
		t.Fatalf("expected explicit zero trace sample rate to be preserved, got %v", cfg.Tracing.SampleRate)
	}
}

func TestValidateConfig_RequiresLoggingExportEndpointWhenOTLPEnabled(t *testing.T) {
	err := ValidateConfig(Config{
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

func TestValidateConfig_RequiresMetricsEndpointWhenEnabled(t *testing.T) {
	err := ValidateConfig(Config{
		Metrics: runtime.MetricsConfig{Enabled: true},
	})
	if err == nil {
		t.Fatal("expected metrics validation error")
	}
	if !strings.Contains(err.Error(), "Endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
}

func TestValidateConfig_RejectsNonIncreasingMetricBuckets(t *testing.T) {
	err := ValidateConfig(Config{
		Metrics: runtime.MetricsConfig{
			HistogramBuckets: runtime.HistogramBuckets{
				FlowMS: []float64{10, 25, 25},
			},
		},
	})
	if err == nil {
		t.Fatal("expected bucket validation error")
	}
	if !strings.Contains(err.Error(), "FlowMS") {
		t.Fatalf("expected FlowMS validation error, got %v", err)
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

func TestInitLoggerWithWriter_FansOutToStdoutAndOTLP(t *testing.T) {
	var stdout bytes.Buffer
	var otlp bytes.Buffer

	prev := newOTLPLogHandler
	t.Cleanup(func() { newOTLPLogHandler = prev })
	newOTLPLogHandler = func(cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
		return slog.NewJSONHandler(&otlp, &slog.HandlerOptions{Level: slog.LevelDebug}), func(context.Context) error { return nil }, nil
	}

	logger, shutdown, err := InitLoggerWithWriter(&stdout, Config{
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
		t.Fatalf("InitLoggerWithWriter failed: %v", err)
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

func TestInitLoggerWithWriter_OTLPOnlySkipsStdout(t *testing.T) {
	var stdout bytes.Buffer
	var otlp bytes.Buffer

	prev := newOTLPLogHandler
	t.Cleanup(func() { newOTLPLogHandler = prev })
	newOTLPLogHandler = func(cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
		return slog.NewJSONHandler(&otlp, &slog.HandlerOptions{Level: slog.LevelDebug}), func(context.Context) error { return nil }, nil
	}

	logger, shutdown, err := InitLoggerWithWriter(&stdout, Config{
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
		t.Fatalf("InitLoggerWithWriter failed: %v", err)
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
	initObservabilityLogger = func(cfg Config) (*slog.Logger, func(context.Context) error, error) {
		return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(context.Context) error {
			loggerShutdownCalled = true
			return nil
		}, nil
	}
	initTracingRuntime = func(cfg TracingConfig) (trace.Tracer, func(context.Context) error, error) {
		return nil, nil, errors.New("tracing boom")
	}
	initMetricsRuntime = func(cfg runtime.MetricsConfig) (*runtime.Metrics, func(context.Context) error, error) {
		t.Fatal("metrics init should not be called when tracing init fails")
		return nil, nil, nil
	}

	_, err := Init(Config{})
	if err == nil {
		t.Fatal("expected InitObservability to fail")
	}
	if !loggerShutdownCalled {
		t.Fatal("expected logger shutdown cleanup when tracing init fails")
	}
}
