package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

const defaultLogPayloadLimit = 10 * 1024

type ObservabilityConfig struct {
	Logging LoggingConfig `yaml:"logging"`
	Tracing TracingConfig `yaml:"tracing"`
	Metrics MetricsConfig `yaml:"metrics"`
}

type LoggingConfig struct {
	Level           string           `yaml:"level" default:"info" validate:"omitempty,oneof=debug info warn error"`
	Format          string           `yaml:"format" default:"json" validate:"omitempty,oneof=json"`
	MaxPayloadBytes int              `yaml:"max_payload_bytes" default:"10240" validate:"gte=0,lte=1048576"`
	Attributes      map[string]any   `yaml:"attributes,omitempty"`
	Sources         LogSourcesConfig `yaml:"sources,omitempty"`
	Masking         MaskingConfig    `yaml:"masking,omitempty"`
	Export          LogExportConfig  `yaml:"export,omitempty"`
}

type LogExportConfig struct {
	Enabled    bool              `yaml:"enabled" default:"false"`
	Mode       LogExportModes    `yaml:"mode,omitempty"`
	Endpoint   string            `yaml:"endpoint,omitempty" validate:"omitempty,hostname_port"`
	Insecure   bool              `yaml:"insecure" default:"false"`
	Attributes map[string]string `yaml:"attributes,omitempty"`
}

type LogExportModes []string

const (
	logExportModeStdout = "stdout"
	logExportModeOTLP   = "otlp"
)

func (m *LogExportModes) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*m = nil
		return nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		var value string
		if err := node.Decode(&value); err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			*m = nil
			return nil
		}
		*m = LogExportModes{value}
		return nil
	case yaml.SequenceNode:
		var values []string
		if err := node.Decode(&values); err != nil {
			return err
		}
		*m = LogExportModes(values)
		return nil
	default:
		return fmt.Errorf("log export mode must be a string or list of strings")
	}
}

type LogSourcesConfig struct {
	Framework string `yaml:"framework,omitempty" validate:"omitempty,oneof=debug info warn error"`
	Plugin    string `yaml:"plugin,omitempty" validate:"omitempty,oneof=debug info warn error"`
	User      string `yaml:"user,omitempty" validate:"omitempty,oneof=debug info warn error"`
}

type MaskingConfig struct {
	Fields      []string `yaml:"fields,omitempty"`
	Placeholder string   `yaml:"placeholder,omitempty" default:"***"`
}

type TracingConfig struct {
	Enabled    bool              `yaml:"enabled" default:"false"`
	Endpoint   string            `yaml:"endpoint,omitempty" validate:"omitempty,hostname_port"`
	Insecure   bool              `yaml:"insecure" default:"false"`
	Sampler    string            `yaml:"sampler" default:"always_on" validate:"omitempty,oneof=always_on always_off trace_id_ratio parent_based"`
	SampleRate float64           `yaml:"sample_rate" validate:"gte=0,lte=1"`
	Attributes map[string]string `yaml:"attributes,omitempty"`

	sampleRateSet bool `yaml:"-"`
}

type MetricsConfig struct {
	Enabled          bool              `yaml:"enabled" default:"false"`
	Endpoint         string            `yaml:"endpoint,omitempty" validate:"omitempty,hostname_port"`
	Insecure         bool              `yaml:"insecure" default:"false"`
	ExportIntervalMS int               `yaml:"export_interval_ms" default:"10000" validate:"omitempty,gte=1000,lte=60000"`
	Attributes       map[string]string `yaml:"attributes,omitempty"`
	HistogramBuckets HistogramBuckets  `yaml:"histogram_buckets,omitempty"`
	User             UserMetricsConfig `yaml:"user,omitempty"`
}

type HistogramBuckets struct {
	HTTPRequestMS []float64 `yaml:"http_request_ms,omitempty"`
	FlowMS        []float64 `yaml:"flow_ms,omitempty"`
	StepMS        []float64 `yaml:"step_ms,omitempty"`
	PluginMS      []float64 `yaml:"plugin_ms,omitempty"`
}

// UserMetricsConfig holds optional user-defined metric declarations.
// Dynamic metrics (metric.counter, metric.histogram, etc.) are always available.
// Predeclared metrics provide startup validation and named handles.
type UserMetricsConfig struct {
	Declarations map[string]UserMetricDecl `yaml:"declarations,omitempty"`
}

// UserMetricDecl defines a predeclared user metric.
type UserMetricDecl struct {
	Type        string                     `yaml:"type"`
	Unit        string                     `yaml:"unit,omitempty"`
	Description string                     `yaml:"description,omitempty"`
	Buckets     []float64                  `yaml:"buckets,omitempty"`
	Labels      map[string]UserMetricLabel `yaml:"labels,omitempty"`
}

// UserMetricLabel defines a label constraint for a predeclared metric.
type UserMetricLabel struct {
	Type   string   `yaml:"type"`
	Values []string `yaml:"values,omitempty"`
}

type observabilityContext interface {
	observabilityAttrs() []slog.Attr
}

func NewObservabilityLogger(cfg ObservabilityConfig) *slog.Logger {
	logger, _, err := InitObservabilityLoggerWithWriter(os.Stdout, cfg)
	if err == nil {
		return logger
	}
	fmt.Fprintf(os.Stderr, "sflowg: failed to initialize log export, falling back to stdout-only: %v\n", err)
	fallback := cfg
	fallback.Logging.Export = LogExportConfig{}
	return NewObservabilityLoggerWithWriter(os.Stdout, fallback)
}

func NewObservabilityLoggerWithWriter(w io.Writer, cfg ObservabilityConfig) *slog.Logger {
	handler := newObservabilityHandler(w, cfg.Logging, "framework")
	logger := slog.New(handler)
	if len(cfg.Logging.Attributes) > 0 {
		attrs := make([]any, 0, len(cfg.Logging.Attributes)*2)
		for key, value := range cfg.Logging.Attributes {
			attrs = append(attrs, key, value)
		}
		logger = logger.With(attrs...)
	}
	return logger
}

func InitObservabilityLogger(cfg ObservabilityConfig) (*slog.Logger, func(context.Context) error, error) {
	return InitObservabilityLoggerWithWriter(os.Stdout, cfg)
}

func InitObservabilityLoggerWithWriter(w io.Writer, cfg ObservabilityConfig) (*slog.Logger, func(context.Context) error, error) {
	handler, shutdown, err := newObservabilityHandlerWithShutdown(w, cfg.Logging, "framework")
	if err != nil {
		return nil, nil, err
	}

	logger := slog.New(handler)
	if len(cfg.Logging.Attributes) > 0 {
		attrs := make([]any, 0, len(cfg.Logging.Attributes)*2)
		for key, value := range cfg.Logging.Attributes {
			attrs = append(attrs, key, value)
		}
		logger = logger.With(attrs...)
	}

	return logger, shutdown, nil
}

func DefaultObservabilityConfig() ObservabilityConfig {
	cfg := ObservabilityConfig{}
	_ = ApplyObservabilityDefaults(&cfg)
	return cfg
}

func ApplyObservabilityDefaults(cfg *ObservabilityConfig) error {
	if err := ApplyDefaults(cfg); err != nil {
		return err
	}
	if !cfg.Tracing.sampleRateSet && cfg.Tracing.SampleRate == 0 {
		cfg.Tracing.SampleRate = 1.0
	}
	cfg.Logging.Export.Mode = normalizeLogExportModes(cfg.Logging.Export.Mode)
	return nil
}

func (c *TracingConfig) UnmarshalYAML(node *yaml.Node) error {
	type rawTracingConfig TracingConfig

	var raw rawTracingConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}

	*c = TracingConfig(raw)
	c.sampleRateSet = yamlMappingHasKey(node, "sample_rate")
	return nil
}

func ValidateObservabilityConfig(cfg ObservabilityConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	tracing := cfg.Tracing
	if tracing.Enabled && strings.TrimSpace(tracing.Endpoint) == "" {
		return fmt.Errorf("config validation failed:\n  - field 'Endpoint' is required when tracing is enabled")
	}

	sampler := tracing.Sampler
	if sampler == "" {
		sampler = "always_on"
	}
	if sampler != "trace_id_ratio" && sampler != "parent_based" && tracing.SampleRate != 0 && tracing.SampleRate != 1 {
		return fmt.Errorf("config validation failed:\n  - field 'SampleRate' is only used with trace_id_ratio or parent_based samplers")
	}

	metrics := cfg.Metrics
	if metrics.Enabled && strings.TrimSpace(metrics.Endpoint) == "" {
		return fmt.Errorf("config validation failed:\n  - field 'Endpoint' is required when metrics is enabled")
	}
	if err := validateIncreasingBuckets("HTTPRequestMS", metrics.HistogramBuckets.HTTPRequestMS); err != nil {
		return err
	}
	if err := validateIncreasingBuckets("FlowMS", metrics.HistogramBuckets.FlowMS); err != nil {
		return err
	}
	if err := validateIncreasingBuckets("StepMS", metrics.HistogramBuckets.StepMS); err != nil {
		return err
	}
	if err := validateIncreasingBuckets("PluginMS", metrics.HistogramBuckets.PluginMS); err != nil {
		return err
	}

	if err := validateUserMetricsConfig(metrics.User); err != nil {
		return err
	}

	logExport := cfg.Logging.Export
	modes := normalizeLogExportModes(logExport.Mode)
	if err := validateLogExportModes(modes); err != nil {
		return err
	}
	if slices.Contains(modes, logExportModeOTLP) {
		if !logExport.Enabled {
			return fmt.Errorf("config validation failed:\n  - field 'Enabled' must be true when logging.export.mode includes otlp")
		}
		if strings.TrimSpace(logExport.Endpoint) == "" {
			return fmt.Errorf("config validation failed:\n  - field 'Endpoint' is required when logging.export.mode includes otlp")
		}
	}

	return nil
}

func normalizeLogExportModes(modes LogExportModes) LogExportModes {
	if len(modes) == 0 {
		return LogExportModes{logExportModeStdout}
	}

	normalized := make(LogExportModes, 0, len(modes))
	seen := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		value := strings.ToLower(strings.TrimSpace(mode))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return LogExportModes{logExportModeStdout}
	}
	return normalized
}

func yamlMappingHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func validateLogExportModes(modes LogExportModes) error {
	for _, mode := range modes {
		switch mode {
		case logExportModeStdout, logExportModeOTLP:
		default:
			return fmt.Errorf("config validation failed:\n  - field 'Mode' must contain only stdout or otlp")
		}
	}
	return nil
}

var validMetricNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

const (
	maxMetricNameLen = 128
	maxLabelNameLen  = 64
)

var validUserMetricTypes = map[string]bool{
	"counter":       true,
	"updowncounter": true,
	"histogram":     true,
	"gauge":         true,
}

// reservedUserMetricDSLNames are the names used by the dynamic metric API
// (metric.counter, metric.histogram, etc.). Predeclared metric declarations
// must not use these names or they would overwrite the API functions in the
// DSL metric module, silently breaking metric.counter(...) and siblings.
var reservedUserMetricDSLNames = map[string]bool{
	"counter":       true,
	"updowncounter": true,
	"histogram":     true,
	"gauge":         true,
}

func validateUserMetricsConfig(cfg UserMetricsConfig) error {
	for name, decl := range cfg.Declarations {
		if err := validateUserMetricDecl(name, decl); err != nil {
			return err
		}
	}
	return nil
}

func validateUserMetricDecl(name string, decl UserMetricDecl) error {
	if name == "" {
		return fmt.Errorf("config validation failed:\n  - user metric name must not be empty")
	}
	if len(name) > maxMetricNameLen {
		return fmt.Errorf("config validation failed:\n  - user metric name %q exceeds maximum length %d", name, maxMetricNameLen)
	}
	if !validMetricNamePattern.MatchString(name) {
		return fmt.Errorf("config validation failed:\n  - user metric name %q contains invalid characters", name)
	}
	if reservedUserMetricDSLNames[name] {
		return fmt.Errorf("config validation failed:\n  - user metric name %q is reserved by the DSL metric API", name)
	}
	if !validUserMetricTypes[decl.Type] {
		return fmt.Errorf("config validation failed:\n  - user metric %q has invalid type %q, must be one of: counter, updowncounter, histogram, gauge", name, decl.Type)
	}
	if len(decl.Buckets) > 0 && decl.Type != "histogram" {
		return fmt.Errorf("config validation failed:\n  - user metric %q has buckets but type is %q (buckets are only valid for histogram)", name, decl.Type)
	}
	if err := validateIncreasingBuckets(fmt.Sprintf("user.%s.buckets", name), decl.Buckets); err != nil {
		return err
	}
	for labelName, label := range decl.Labels {
		if err := validateUserMetricLabel(name, labelName, label); err != nil {
			return err
		}
	}
	return nil
}

func validateUserMetricLabel(metricName, labelName string, label UserMetricLabel) error {
	if labelName == "" {
		return fmt.Errorf("config validation failed:\n  - user metric %q has empty label name", metricName)
	}
	if len(labelName) > maxLabelNameLen {
		return fmt.Errorf("config validation failed:\n  - user metric %q label %q exceeds maximum length %d", metricName, labelName, maxLabelNameLen)
	}
	if !validMetricNamePattern.MatchString(labelName) {
		return fmt.Errorf("config validation failed:\n  - user metric %q label %q contains invalid characters", metricName, labelName)
	}
	if label.Type != "enum" {
		return fmt.Errorf("config validation failed:\n  - user metric %q label %q has invalid type %q, must be enum", metricName, labelName, label.Type)
	}
	if len(label.Values) == 0 {
		return fmt.Errorf("config validation failed:\n  - user metric %q label %q must have at least one enum value", metricName, labelName)
	}
	return nil
}

func validateIncreasingBuckets(field string, values []float64) error {
	for i := 1; i < len(values); i++ {
		if values[i] <= values[i-1] {
			return fmt.Errorf("config validation failed:\n  - field '%s' must be strictly increasing", field)
		}
	}
	return nil
}

func parseLogLevel(level string, fallback slog.Level) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return fallback
	}
}

type observabilityHandler struct {
	next            slog.Handler
	source          string // baked-in source identity: "framework", "plugin", "user", or ""
	defaultLevel    slog.Level
	enabledLevel    slog.Level // = sourceLevels[source] || defaultLevel
	sourceLevels    map[string]slog.Level
	maskFields      []string
	maskPlaceholder string
	maxPayloadBytes int
	boundAttrs      []slog.Attr
}

func newObservabilityHandler(w io.Writer, cfg LoggingConfig, source string) slog.Handler {
	handler, _, err := newObservabilityHandlerWithShutdown(w, cfg, source)
	if err == nil {
		return handler
	}

	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	base := slog.NewJSONHandler(w, opts)
	return newObservabilityHandlerWithBase(base, cfg, source)
}

func newObservabilityHandlerWithShutdown(w io.Writer, cfg LoggingConfig, source string) (slog.Handler, func(context.Context) error, error) {
	base, shutdown, err := newObservabilityBaseHandler(w, cfg)
	if err != nil {
		return nil, nil, err
	}
	return newObservabilityHandlerWithBase(base, cfg, source), shutdown, nil
}

func newObservabilityHandlerWithBase(base slog.Handler, cfg LoggingConfig, source string) slog.Handler {
	maxPayloadBytes := cfg.MaxPayloadBytes
	if maxPayloadBytes == 0 {
		maxPayloadBytes = defaultLogPayloadLimit
	}

	maskPlaceholder := cfg.Masking.Placeholder
	if maskPlaceholder == "" {
		maskPlaceholder = "***"
	}

	defaultLevel := parseLogLevel(cfg.Level, slog.LevelInfo)
	sourceLevels := map[string]slog.Level{
		"framework": parseLogLevel(cfg.Sources.Framework, defaultLevel),
		"plugin":    parseLogLevel(cfg.Sources.Plugin, defaultLevel),
		"user":      parseLogLevel(cfg.Sources.User, defaultLevel),
	}

	enabledLevel := defaultLevel
	if l, ok := sourceLevels[source]; ok {
		enabledLevel = l
	}

	return &observabilityHandler{
		next:            base,
		source:          source,
		defaultLevel:    defaultLevel,
		enabledLevel:    enabledLevel,
		sourceLevels:    sourceLevels,
		maskFields:      normalizeMaskFields(cfg.Masking.Fields),
		maskPlaceholder: maskPlaceholder,
		maxPayloadBytes: maxPayloadBytes,
	}
}

// clone returns a shallow copy with the given next handler and bound attrs.
// Shared slices (maskFields, sourceLevels) are safe to alias — they are read-only after init.
func (h *observabilityHandler) clone(next slog.Handler, boundAttrs []slog.Attr) *observabilityHandler {
	return &observabilityHandler{
		next:            next,
		source:          h.source,
		defaultLevel:    h.defaultLevel,
		enabledLevel:    h.enabledLevel,
		sourceLevels:    h.sourceLevels,
		maskFields:      h.maskFields,
		maskPlaceholder: h.maskPlaceholder,
		maxPayloadBytes: h.maxPayloadBytes,
		boundAttrs:      boundAttrs,
	}
}

// withSource returns a copy of the handler with a different source identity and
// its corresponding source-level filter. Used by Logger.ForPlugin and Logger.ForUser.
func (h *observabilityHandler) withSource(source string) *observabilityHandler {
	enabledLevel := h.defaultLevel
	if l, ok := h.sourceLevels[source]; ok {
		enabledLevel = l
	}
	c := h.clone(h.next, nil)
	c.source = source
	c.enabledLevel = enabledLevel
	return c
}

func (h *observabilityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.enabledLevel
}

func (h *observabilityHandler) Handle(ctx context.Context, r slog.Record) error {
	record := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	r.Attrs(func(attr slog.Attr) bool {
		record.AddAttrs(h.sanitizeAttr(attr))
		return true
	})

	// Inject source from handler identity — appears exactly once, never duplicated.
	if h.source != "" {
		record.AddAttrs(slog.String("source", h.source))
	}

	if carrier, ok := ctx.(observabilityContext); ok {
		record.AddAttrs(carrier.observabilityAttrs()...)
	}

	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return h.next.Handle(ctx, record)
}

func (h *observabilityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	boundAttrs := append([]slog.Attr{}, h.boundAttrs...)
	boundAttrs = append(boundAttrs, attrs...)
	return h.clone(h.next.WithAttrs(attrs), boundAttrs)
}

func (h *observabilityHandler) WithGroup(name string) slog.Handler {
	return h.clone(h.next.WithGroup(name), append([]slog.Attr{}, h.boundAttrs...))
}

func (h *observabilityHandler) sanitizeAttr(attr slog.Attr) slog.Attr {
	sanitized, changed := h.sanitizeAttrValue(attr)
	if !changed {
		return attr
	}
	attr.Value = sanitized
	return attr
}

func (h *observabilityHandler) sanitizeAttrValue(attr slog.Attr) (slog.Value, bool) {
	sanitized, changed := h.sanitizeValue(attr.Key, attr.Value)
	if !changed {
		return attr.Value, false
	}
	return sanitized, true
}

func (h *observabilityHandler) sanitizeValue(key string, value slog.Value) (slog.Value, bool) {
	if h.shouldMask(key) {
		return slog.StringValue(h.maskPlaceholder), true
	}

	switch value.Kind() {
	case slog.KindString:
		truncated := truncateString(value.String(), h.maxPayloadBytes)
		if truncated == value.String() {
			return value, false
		}
		return slog.StringValue(truncated), true
	case slog.KindGroup:
		group := value.Group()
		sanitized := make([]slog.Attr, len(group))
		changed := false
		for i, attr := range group {
			sanitizedValue, attrChanged := h.sanitizeAttrValue(attr)
			sanitized[i] = attr
			sanitized[i].Value = sanitizedValue
			changed = changed || attrChanged
		}
		if !changed {
			return value, false
		}
		return slog.GroupValue(sanitized...), true
	case slog.KindLogValuer:
		resolved := value.Resolve()
		sanitized, changed := h.sanitizeValue(key, resolved)
		if !changed {
			return value, false
		}
		return sanitized, true
	case slog.KindAny:
		sanitized, changed := h.sanitizeAnyValue(key, value.Any())
		if !changed {
			return value, false
		}
		return slog.AnyValue(sanitized), true
	default:
		return value, false
	}
}

func (h *observabilityHandler) sanitizeAnyValue(key string, value any) (any, bool) {
	if value == nil {
		return nil, false
	}

	switch v := value.(type) {
	case string:
		truncated := truncateString(v, h.maxPayloadBytes)
		return truncated, truncated != v
	case error:
		s := v.Error()
		truncated := truncateString(s, h.maxPayloadBytes)
		return truncated, truncated != s
	case fmt.Stringer:
		s := v.String()
		truncated := truncateString(s, h.maxPayloadBytes)
		return truncated, truncated != s
	case []byte:
		s := string(v)
		truncated := truncateString(s, h.maxPayloadBytes)
		return truncated, truncated != s
	case map[string]any:
		sanitized := make(map[string]any, len(v))
		changed := false
		for nestedKey, nestedValue := range v {
			nestedSanitized, nestedChanged := h.sanitizeAnyValue(nestedKey, nestedValue)
			sanitized[nestedKey] = nestedSanitized
			changed = changed || nestedChanged
		}
		if !changed {
			return value, false
		}
		return sanitized, true
	case []any:
		sanitized := make([]any, len(v))
		changed := false
		for i, item := range v {
			nestedSanitized, nestedChanged := h.sanitizeAnyValue(key, item)
			sanitized[i] = nestedSanitized
			changed = changed || nestedChanged
		}
		if !changed {
			return value, false
		}
		return sanitized, true
	default:
		return value, false
	}
}

func (h *observabilityHandler) shouldMask(key string) bool {
	for _, field := range h.maskFields {
		if strings.EqualFold(field, key) {
			return true
		}
	}
	return false
}

func normalizeMaskFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(fields))
	for _, field := range fields {
		normalized = append(normalized, strings.ToLower(field))
	}
	return normalized
}

func truncateString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	suffix := "...[truncated]"
	limit := maxBytes - len(suffix)
	if limit <= 0 {
		return suffix
	}

	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	if limit <= 0 {
		return suffix
	}

	return value[:limit] + suffix
}
