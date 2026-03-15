package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	meterName              = instrumentationName
	metricOutcomeSuccess   = "success"
	metricOutcomeError     = "error"
	metricOutcomeTimeout   = "timeout"
	metricStatusClass2xx   = "2xx"
	metricStatusClass4xx   = "4xx"
	metricStatusClass5xx   = "5xx"
	metricUnknownValue     = "unknown"
	defaultMetricsInterval = 10 * time.Second
)

var (
	defaultHTTPRequestBuckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	defaultFlowBuckets        = []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
	defaultStepBuckets        = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500}
	defaultPluginBuckets      = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500}
	noopMetrics               = &Metrics{}
)

type Metrics struct {
	flowExecutions   otelmetric.Int64Counter
	flowDurationMS   otelmetric.Float64Histogram
	stepExecutions   otelmetric.Int64Counter
	stepDurationMS   otelmetric.Float64Histogram
	stepRetries      otelmetric.Int64Counter
	pluginCalls      otelmetric.Int64Counter
	pluginDurationMS otelmetric.Float64Histogram
	httpRequests     otelmetric.Int64Counter
	httpDurationMS   otelmetric.Float64Histogram

	userMetricsState
}

func InitMetrics(cfg MetricsConfig) (*Metrics, func(context.Context) error, error) {
	if !cfg.Enabled {
		return NewNoopMetrics(), func(context.Context) error { return nil }, nil
	}

	exporterOptions := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		exporterOptions = append(exporterOptions, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(context.Background(), exporterOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(metricsExportInterval(cfg.ExportIntervalMS)))
	provider, err := newMeterProvider(cfg, reader)
	if err != nil {
		_ = exporter.Shutdown(context.Background())
		return nil, nil, err
	}

	metrics, err := newMetrics(provider)
	if err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, nil, err
	}

	// Initialize predeclared user metrics if configured.
	if err := metrics.InitUserMetrics(cfg.User.Declarations); err != nil {
		_ = provider.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("initialize user metrics: %w", err)
	}

	otel.SetMeterProvider(provider)
	return metrics, provider.Shutdown, nil
}

func NewNoopMetrics() *Metrics {
	return noopMetrics
}

func newMeterProvider(cfg MetricsConfig, reader sdkmetric.Reader) (*sdkmetric.MeterProvider, error) {
	options := []sdkmetric.Option{}
	if reader != nil {
		options = append(options, sdkmetric.WithReader(reader))
	}
	if res, err := otelResource(cfg.Attributes); err != nil {
		return nil, fmt.Errorf("build metrics resource: %w", err)
	} else if res != nil {
		options = append(options, sdkmetric.WithResource(res))
	}

	views := []sdkmetric.View{
		newHistogramView("sflowg.http.server.duration_ms", resolveHistogramBuckets(cfg.HistogramBuckets.HTTPRequestMS, defaultHTTPRequestBuckets)),
		newHistogramView("sflowg.flow.duration_ms", resolveHistogramBuckets(cfg.HistogramBuckets.FlowMS, defaultFlowBuckets)),
		newHistogramView("sflowg.step.duration_ms", resolveHistogramBuckets(cfg.HistogramBuckets.StepMS, defaultStepBuckets)),
		newHistogramView("sflowg.plugin.duration_ms", resolveHistogramBuckets(cfg.HistogramBuckets.PluginMS, defaultPluginBuckets)),
	}
	// Register custom bucket views for predeclared user histograms.
	for name, decl := range cfg.User.Declarations {
		if decl.Type == "histogram" && len(decl.Buckets) > 0 {
			views = append(views, newHistogramView(userMetricPrefix+name, decl.Buckets))
		}
	}
	options = append(options, sdkmetric.WithView(views...))

	return sdkmetric.NewMeterProvider(options...), nil
}

func newMetrics(provider otelmetric.MeterProvider) (*Metrics, error) {
	meter := provider.Meter(meterName)

	flowExecutions, err := meter.Int64Counter(
		"sflowg.flow.executions",
		otelmetric.WithDescription("Total number of completed flow executions."),
	)
	if err != nil {
		return nil, fmt.Errorf("create flow execution counter: %w", err)
	}
	flowDurationMS, err := meter.Float64Histogram(
		"sflowg.flow.duration_ms",
		otelmetric.WithDescription("Flow execution duration in milliseconds."),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("create flow duration histogram: %w", err)
	}
	stepExecutions, err := meter.Int64Counter(
		"sflowg.step.executions",
		otelmetric.WithDescription("Total number of completed step executions."),
	)
	if err != nil {
		return nil, fmt.Errorf("create step execution counter: %w", err)
	}
	stepDurationMS, err := meter.Float64Histogram(
		"sflowg.step.duration_ms",
		otelmetric.WithDescription("Step execution duration in milliseconds."),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("create step duration histogram: %w", err)
	}
	stepRetries, err := meter.Int64Counter(
		"sflowg.step.retries",
		otelmetric.WithDescription("Total number of step retries initiated by the runtime."),
	)
	if err != nil {
		return nil, fmt.Errorf("create step retry counter: %w", err)
	}
	pluginCalls, err := meter.Int64Counter(
		"sflowg.plugin.calls",
		otelmetric.WithDescription("Total number of plugin calls."),
	)
	if err != nil {
		return nil, fmt.Errorf("create plugin call counter: %w", err)
	}
	pluginDurationMS, err := meter.Float64Histogram(
		"sflowg.plugin.duration_ms",
		otelmetric.WithDescription("Plugin call duration in milliseconds."),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("create plugin duration histogram: %w", err)
	}
	httpRequests, err := meter.Int64Counter(
		"sflowg.http.server.requests",
		otelmetric.WithDescription("Total number of handled HTTP requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request counter: %w", err)
	}
	httpDurationMS, err := meter.Float64Histogram(
		"sflowg.http.server.duration_ms",
		otelmetric.WithDescription("HTTP request duration in milliseconds."),
		otelmetric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP duration histogram: %w", err)
	}

	return &Metrics{
		flowExecutions:   flowExecutions,
		flowDurationMS:   flowDurationMS,
		stepExecutions:   stepExecutions,
		stepDurationMS:   stepDurationMS,
		stepRetries:      stepRetries,
		pluginCalls:      pluginCalls,
		pluginDurationMS: pluginDurationMS,
		httpRequests:     httpRequests,
		httpDurationMS:   httpDurationMS,
		userMetricsState: userMetricsState{
			meter: meter,
		},
	}, nil
}

func (m *Metrics) RecordFlow(ctx context.Context, flowID string, outcome string, duration time.Duration) {
	if m.flowExecutions == nil || m.flowDurationMS == nil {
		return
	}

	attrs := m.flowAttributes(flowID, outcome)
	m.flowExecutions.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	m.flowDurationMS.Record(ctx, durationMilliseconds(duration), otelmetric.WithAttributes(attrs...))
}

func (m *Metrics) RecordStep(ctx context.Context, flowID string, stepID string, path string, outcome string, duration time.Duration) {
	if m.stepExecutions == nil || m.stepDurationMS == nil {
		return
	}

	attrs := m.stepAttributes(flowID, stepID, path, outcome)
	m.stepExecutions.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	m.stepDurationMS.Record(ctx, durationMilliseconds(duration), otelmetric.WithAttributes(attrs...))
}

func (m *Metrics) RecordRetry(ctx context.Context, flowID string, stepID string, path string) {
	if m.stepRetries == nil {
		return
	}

	attrs := m.retryAttributes(flowID, stepID, path)
	m.stepRetries.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
}

func (m *Metrics) RecordPluginCall(ctx context.Context, flowID string, stepID string, pluginName string, method string, outcome string, duration time.Duration) {
	if m.pluginCalls == nil || m.pluginDurationMS == nil {
		return
	}

	attrs := m.pluginAttributes(flowID, stepID, pluginName, method, outcome)
	m.pluginCalls.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	m.pluginDurationMS.Record(ctx, durationMilliseconds(duration), otelmetric.WithAttributes(attrs...))
}

func (m *Metrics) RecordHTTPRequest(ctx context.Context, flowID string, method string, route string, statusClass string, duration time.Duration) {
	if m.httpRequests == nil || m.httpDurationMS == nil {
		return
	}

	attrs := m.httpAttributes(flowID, method, route, statusClass)
	m.httpRequests.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	m.httpDurationMS.Record(ctx, durationMilliseconds(duration), otelmetric.WithAttributes(attrs...))
}

func (m *Metrics) flowAttributes(flowID string, outcome string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("flow.id", normalizeMetricValue(flowID)),
		attribute.String("outcome", normalizeOutcome(outcome)),
	}
}

func (m *Metrics) stepAttributes(flowID string, stepID string, path string, outcome string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("flow.id", normalizeMetricValue(flowID)),
		attribute.String("step.id", normalizeMetricValue(stepID)),
		attribute.String("path", normalizePath(path)),
		attribute.String("outcome", normalizeOutcome(outcome)),
	}
}

func (m *Metrics) retryAttributes(flowID string, stepID string, path string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("flow.id", normalizeMetricValue(flowID)),
		attribute.String("step.id", normalizeMetricValue(stepID)),
		attribute.String("path", normalizePath(path)),
	}
}

func (m *Metrics) pluginAttributes(flowID string, stepID string, pluginName string, method string, outcome string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("flow.id", normalizeMetricValue(flowID)),
		attribute.String("plugin.name", normalizeMetricValue(pluginName)),
		attribute.String("plugin.method", normalizeMetricValue(method)),
		attribute.String("outcome", normalizeOutcome(outcome)),
	}
	if strings.TrimSpace(stepID) != "" {
		attrs = append(attrs, attribute.String("step.id", normalizeMetricValue(stepID)))
	}
	return attrs
}

func (m *Metrics) httpAttributes(flowID string, method string, route string, statusClass string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("flow.id", normalizeMetricValue(flowID)),
		attribute.String("http.method", normalizeHTTPMethod(method)),
		attribute.String("http.route", normalizeMetricValue(route)),
		attribute.String("http.status_class", normalizeHTTPStatusClass(statusClass)),
	}
}

func newHistogramView(name string, boundaries []float64) sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{
			Name: name,
			Kind: sdkmetric.InstrumentKindHistogram,
		},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: boundaries,
			},
		},
	)
}

func resolveHistogramBuckets(values []float64, defaults []float64) []float64 {
	if len(values) == 0 {
		return append([]float64(nil), defaults...)
	}
	return append([]float64(nil), values...)
}

func metricsExportInterval(value int) time.Duration {
	if value <= 0 {
		return defaultMetricsInterval
	}
	return time.Duration(value) * time.Millisecond
}

func classifyMetricOutcome(err error) string {
	if err == nil {
		return metricOutcomeSuccess
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return metricOutcomeTimeout
	}

	var flowErr *FlowError
	if errors.As(err, &flowErr) {
		switch flowErr.Type {
		case ErrorTypeTimeout:
			return metricOutcomeTimeout
		default:
			if flowErr.Code == string(ErrorCodeDeadlineExceeded) || flowErr.Code == string(ErrorCodeContextCancelled) {
				return metricOutcomeTimeout
			}
		}
	}

	return metricOutcomeError
}

func classifyHTTPStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return metricStatusClass2xx
	case statusCode >= 400 && statusCode < 500:
		return metricStatusClass4xx
	case statusCode >= 500 && statusCode < 600:
		return metricStatusClass5xx
	default:
		return metricUnknownValue
	}
}

func normalizeMetricValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return metricUnknownValue
	}
	return value
}

func normalizeOutcome(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case metricOutcomeSuccess:
		return metricOutcomeSuccess
	case metricOutcomeError:
		return metricOutcomeError
	case metricOutcomeTimeout:
		return metricOutcomeTimeout
	default:
		return metricUnknownValue
	}
}

func normalizePath(value string) string {
	switch SuccessPath(strings.ToLower(strings.TrimSpace(value))) {
	case SuccessPathPrimary:
		return string(SuccessPathPrimary)
	case SuccessPathFallback:
		return string(SuccessPathFallback)
	default:
		return metricUnknownValue
	}
}

func normalizeHTTPMethod(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return metricUnknownValue
	}
	return value
}

func normalizeHTTPStatusClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case metricStatusClass2xx:
		return metricStatusClass2xx
	case metricStatusClass4xx:
		return metricStatusClass4xx
	case metricStatusClass5xx:
		return metricStatusClass5xx
	default:
		return metricUnknownValue
	}
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
