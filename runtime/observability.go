package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/trace"
)

const defaultLogPayloadLimit = 10 * 1024

type ObservabilityConfig struct {
	Logging LoggingConfig  `yaml:"logging"`
	Tracing TracingConfig  `yaml:"tracing"`
	Metrics []MetricConfig `yaml:"metrics,omitempty"`
}

type LoggingConfig struct {
	Level           string           `yaml:"level" default:"info" validate:"omitempty,oneof=debug info warn error"`
	Format          string           `yaml:"format" default:"json" validate:"omitempty,oneof=json"`
	MaxPayloadBytes int              `yaml:"max_payload_bytes" default:"10240" validate:"gte=0,lte=1048576"`
	Attributes      map[string]any   `yaml:"attributes,omitempty"`
	Sources         LogSourcesConfig `yaml:"sources,omitempty"`
	Masking         MaskingConfig    `yaml:"masking,omitempty"`
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
	SampleRate float64           `yaml:"sample_rate" default:"1.0" validate:"gte=0,lte=1"`
	Attributes map[string]string `yaml:"attributes,omitempty"`
}

type observabilityContext interface {
	observabilityAttrs() []slog.Attr
}

func NewObservabilityLogger(cfg ObservabilityConfig) *slog.Logger {
	return NewObservabilityLoggerWithWriter(os.Stdout, cfg)
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

func DefaultObservabilityConfig() ObservabilityConfig {
	cfg := ObservabilityConfig{}
	_ = ApplyObservabilityDefaults(&cfg)
	return cfg
}

func ApplyObservabilityDefaults(cfg *ObservabilityConfig) error {
	return ApplyDefaults(cfg)
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
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	base := slog.NewJSONHandler(w, opts)

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
	if len(h.maskFields) == 0 {
		return false
	}
	return slices.Contains(h.maskFields, strings.ToLower(key))
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

	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	if limit <= 0 {
		return suffix
	}

	return value[:limit] + suffix
}
