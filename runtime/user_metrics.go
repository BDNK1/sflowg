package runtime

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const (
	userMetricPrefix     = "sflowg.user."
	maxLabelKeyLen       = 64
	maxLabelValueLen     = 256
	maxLabelsPerMetric   = 10
)

// userMetricsState holds the user metrics registries and config.
// Embedded in Metrics to extend it without modifying the platform metric struct fields.
type userMetricsState struct {
	meter otelmetric.Meter // stored from init for lazy dynamic creation

	// Dynamic registries (sync.Map for concurrent lazy creation).
	dynamicCounters       sync.Map // string → otelmetric.Float64Counter
	dynamicUpDownCounters sync.Map // string → otelmetric.Float64UpDownCounter
	dynamicHistograms     sync.Map // string → otelmetric.Float64Histogram
	dynamicGauges         sync.Map // string → otelmetric.Float64Gauge
	dynamicTypes          sync.Map // string → string (name → type for conflict detection)

	// Predeclared (created eagerly at startup, no sync needed).
	predeclaredCounters       map[string]otelmetric.Float64Counter
	predeclaredUpDownCounters map[string]otelmetric.Float64UpDownCounter
	predeclaredHistograms     map[string]otelmetric.Float64Histogram
	predeclaredGauges         map[string]otelmetric.Float64Gauge
	userDecls                 map[string]UserMetricDecl

	// Auto-attached context from properties.observability.metrics.context.
	userMetricContext map[string]string
}

// UserDeclarations returns the predeclared user metric declarations.
func (m *Metrics) UserDeclarations() map[string]UserMetricDecl {
	if m.userDecls == nil {
		return nil
	}
	return m.userDecls
}

// SetUserMetricContext configures the property-based auto-attached context.
func (m *Metrics) SetUserMetricContext(ctx map[string]string) {
	m.userMetricContext = ctx
}

// InitUserMetrics creates predeclared instruments from config declarations.
// Must be called during startup after the meter is available.
func (m *Metrics) InitUserMetrics(decls map[string]UserMetricDecl) error {
	if m.meter == nil {
		return nil
	}
	m.userDecls = decls
	m.predeclaredCounters = make(map[string]otelmetric.Float64Counter)
	m.predeclaredUpDownCounters = make(map[string]otelmetric.Float64UpDownCounter)
	m.predeclaredHistograms = make(map[string]otelmetric.Float64Histogram)
	m.predeclaredGauges = make(map[string]otelmetric.Float64Gauge)

	for name, decl := range decls {
		if err := m.createPredeclaredInstrument(name, decl); err != nil {
			return fmt.Errorf("create predeclared metric %q: %w", name, err)
		}
	}
	return nil
}

func (m *Metrics) createPredeclaredInstrument(name string, decl UserMetricDecl) error {
	otelName := userMetricPrefix + name
	opts := []otelmetric.Float64CounterOption{}
	if decl.Description != "" {
		opts = append(opts, otelmetric.WithDescription(decl.Description))
	}
	if decl.Unit != "" {
		opts = append(opts, otelmetric.WithUnit(decl.Unit))
	}

	switch decl.Type {
	case "counter":
		inst, err := m.meter.Float64Counter(otelName, opts...)
		if err != nil {
			return err
		}
		m.predeclaredCounters[name] = inst
	case "updowncounter":
		udOpts := make([]otelmetric.Float64UpDownCounterOption, 0, len(opts))
		if decl.Description != "" {
			udOpts = append(udOpts, otelmetric.WithDescription(decl.Description))
		}
		if decl.Unit != "" {
			udOpts = append(udOpts, otelmetric.WithUnit(decl.Unit))
		}
		inst, err := m.meter.Float64UpDownCounter(otelName, udOpts...)
		if err != nil {
			return err
		}
		m.predeclaredUpDownCounters[name] = inst
	case "histogram":
		hOpts := make([]otelmetric.Float64HistogramOption, 0, 3)
		if decl.Description != "" {
			hOpts = append(hOpts, otelmetric.WithDescription(decl.Description))
		}
		if decl.Unit != "" {
			hOpts = append(hOpts, otelmetric.WithUnit(decl.Unit))
		}
		inst, err := m.meter.Float64Histogram(otelName, hOpts...)
		if err != nil {
			return err
		}
		m.predeclaredHistograms[name] = inst
	case "gauge":
		gOpts := make([]otelmetric.Float64GaugeOption, 0, 2)
		if decl.Description != "" {
			gOpts = append(gOpts, otelmetric.WithDescription(decl.Description))
		}
		if decl.Unit != "" {
			gOpts = append(gOpts, otelmetric.WithUnit(decl.Unit))
		}
		inst, err := m.meter.Float64Gauge(otelName, gOpts...)
		if err != nil {
			return err
		}
		m.predeclaredGauges[name] = inst
	default:
		return fmt.Errorf("unknown metric type %q", decl.Type)
	}
	return nil
}

// RecordUserCounter records a user-defined counter metric.
func (m *Metrics) RecordUserCounter(ctx context.Context, exec *Execution, name string, value float64, labels map[string]any) {
	if m.meter == nil {
		return
	}
	logger := userMetricLogger(exec)

	if value <= 0 {
		logger.Warn("dropping counter metric: value must be positive",
			"metric", name, "value", value)
		return
	}

	attrs, ok := m.buildUserMetricAttrs(exec, name, "counter", labels, logger)
	if !ok {
		return
	}

	// Check predeclared first.
	if inst, found := m.predeclaredCounters[name]; found {
		inst.Add(ctx, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst := m.getOrCreateDynamicCounter(name, logger)
	if inst == nil {
		return
	}
	inst.Add(ctx, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserUpDownCounter records a user-defined up-down counter metric.
func (m *Metrics) RecordUserUpDownCounter(ctx context.Context, exec *Execution, name string, value float64, labels map[string]any) {
	if m.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "updowncounter", labels, logger)
	if !ok {
		return
	}

	if inst, found := m.predeclaredUpDownCounters[name]; found {
		inst.Add(ctx, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst := m.getOrCreateDynamicUpDownCounter(name, logger)
	if inst == nil {
		return
	}
	inst.Add(ctx, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserHistogram records a user-defined histogram metric.
func (m *Metrics) RecordUserHistogram(ctx context.Context, exec *Execution, name string, value float64, labels map[string]any) {
	if m.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "histogram", labels, logger)
	if !ok {
		return
	}

	if inst, found := m.predeclaredHistograms[name]; found {
		inst.Record(ctx, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst := m.getOrCreateDynamicHistogram(name, logger)
	if inst == nil {
		return
	}
	inst.Record(ctx, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserGauge records a user-defined gauge metric.
func (m *Metrics) RecordUserGauge(ctx context.Context, exec *Execution, name string, value float64, labels map[string]any) {
	if m.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "gauge", labels, logger)
	if !ok {
		return
	}

	if inst, found := m.predeclaredGauges[name]; found {
		inst.Record(ctx, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst := m.getOrCreateDynamicGauge(name, logger)
	if inst == nil {
		return
	}
	inst.Record(ctx, value, otelmetric.WithAttributes(attrs...))
}

// --- Dynamic instrument creation (sync.Map) ---

func (m *Metrics) getOrCreateDynamicCounter(name string, logger Logger) otelmetric.Float64Counter {
	if existing, ok := m.dynamicCounters.Load(name); ok {
		return existing.(otelmetric.Float64Counter)
	}
	if !m.checkDynamicTypeConflict(name, "counter", logger) {
		return nil
	}
	inst, err := m.meter.Float64Counter(userMetricPrefix + name)
	if err != nil {
		logger.Warn("failed to create dynamic counter", "metric", name, "error", err)
		return nil
	}
	actual, _ := m.dynamicCounters.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Counter)
}

func (m *Metrics) getOrCreateDynamicUpDownCounter(name string, logger Logger) otelmetric.Float64UpDownCounter {
	if existing, ok := m.dynamicUpDownCounters.Load(name); ok {
		return existing.(otelmetric.Float64UpDownCounter)
	}
	if !m.checkDynamicTypeConflict(name, "updowncounter", logger) {
		return nil
	}
	inst, err := m.meter.Float64UpDownCounter(userMetricPrefix + name)
	if err != nil {
		logger.Warn("failed to create dynamic updowncounter", "metric", name, "error", err)
		return nil
	}
	actual, _ := m.dynamicUpDownCounters.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64UpDownCounter)
}

func (m *Metrics) getOrCreateDynamicHistogram(name string, logger Logger) otelmetric.Float64Histogram {
	if existing, ok := m.dynamicHistograms.Load(name); ok {
		return existing.(otelmetric.Float64Histogram)
	}
	if !m.checkDynamicTypeConflict(name, "histogram", logger) {
		return nil
	}
	inst, err := m.meter.Float64Histogram(userMetricPrefix + name)
	if err != nil {
		logger.Warn("failed to create dynamic histogram", "metric", name, "error", err)
		return nil
	}
	actual, _ := m.dynamicHistograms.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Histogram)
}

func (m *Metrics) getOrCreateDynamicGauge(name string, logger Logger) otelmetric.Float64Gauge {
	if existing, ok := m.dynamicGauges.Load(name); ok {
		return existing.(otelmetric.Float64Gauge)
	}
	if !m.checkDynamicTypeConflict(name, "gauge", logger) {
		return nil
	}
	inst, err := m.meter.Float64Gauge(userMetricPrefix + name)
	if err != nil {
		logger.Warn("failed to create dynamic gauge", "metric", name, "error", err)
		return nil
	}
	actual, _ := m.dynamicGauges.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Gauge)
}

func (m *Metrics) checkDynamicTypeConflict(name, metricType string, logger Logger) bool {
	existing, loaded := m.dynamicTypes.LoadOrStore(name, metricType)
	if loaded && existing.(string) != metricType {
		logger.Warn("dropping metric: type conflict",
			"metric", name,
			"existing_type", existing,
			"requested_type", metricType)
		return false
	}
	return true
}

// --- Attribute building ---

// Reserved attribute keys that DSL labels cannot override.
var reservedUserMetricKeys = map[string]bool{
	"flow.id": true,
	"step.id": true,
	"path":    true,
}

func (m *Metrics) buildUserMetricAttrs(exec *Execution, name, metricType string, labels map[string]any, logger Logger) ([]attribute.KeyValue, bool) {
	attrs := make([]attribute.KeyValue, 0, 8+len(labels))

	// Auto-attached execution context.
	if exec.Flow != nil && exec.Flow.ID != "" {
		attrs = append(attrs, attribute.String("flow.id", exec.Flow.ID))
	}
	if stepID := exec.ActiveStepID(); stepID != "" {
		attrs = append(attrs, attribute.String("step.id", stepID))
	}
	if path := exec.ActivePath(); path != "" {
		attrs = append(attrs, attribute.String("path", string(path)))
	}

	// Auto-attached property-based context.
	for k, v := range m.userMetricContext {
		attrs = append(attrs, attribute.String(k, v))
	}

	// Build reserved key set including property-based context keys.
	reserved := make(map[string]bool, len(reservedUserMetricKeys)+len(m.userMetricContext))
	for k := range reservedUserMetricKeys {
		reserved[k] = true
	}
	for k := range m.userMetricContext {
		reserved[k] = true
	}

	// Validate and add user-provided labels.
	decl, isPredeclared := m.userDecls[name]
	labelCount := 0
	for k, v := range labels {
		if reserved[k] {
			logger.Warn("dropping metric: reserved label key used",
				"metric", name, "label", k)
			return nil, false
		}

		labelCount++
		if labelCount > maxLabelsPerMetric {
			logger.Warn("dropping metric: too many labels",
				"metric", name, "max", maxLabelsPerMetric)
			return nil, false
		}

		if len(k) > maxLabelKeyLen {
			logger.Warn("dropping label: key too long",
				"metric", name, "label", k, "max", maxLabelKeyLen)
			continue
		}

		// Predeclared label validation.
		if isPredeclared {
			labelDecl, labelDeclared := decl.Labels[k]
			if !labelDeclared {
				logger.Warn("dropping unknown label for predeclared metric",
					"metric", name, "label", k)
				continue
			}
			strVal := fmt.Sprintf("%v", v)
			if labelDecl.Type == "enum" && !containsValue(labelDecl.Values, strVal) {
				logger.Warn("dropping label: value not in enum",
					"metric", name, "label", k, "value", strVal, "allowed", labelDecl.Values)
				continue
			}
		}

		attr, ok := toScalarAttribute(k, v, logger, name)
		if !ok {
			continue
		}
		attrs = append(attrs, attr)
	}

	return attrs, true
}

func toScalarAttribute(key string, value any, logger Logger, metricName string) (attribute.KeyValue, bool) {
	switch v := value.(type) {
	case string:
		if len(v) > maxLabelValueLen {
			v = v[:maxLabelValueLen]
		}
		return attribute.String(key, v), true
	case int:
		return attribute.Int(key, v), true
	case int64:
		return attribute.Int64(key, v), true
	case float64:
		return attribute.Float64(key, v), true
	case bool:
		return attribute.Bool(key, v), true
	default:
		logger.Warn("dropping label: non-scalar value",
			"metric", metricName, "label", key)
		return attribute.KeyValue{}, false
	}
}

func containsValue(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func userMetricLogger(exec *Execution) Logger {
	if exec == nil || exec.Container == nil {
		return NewLogger(nil)
	}
	return exec.Logger().ForUser()
}

// ValidateUserMetricContext validates that all context values are scalar strings.
func ValidateUserMetricContext(ctx map[string]any) error {
	for k, v := range ctx {
		if k == "" {
			return fmt.Errorf("config validation failed:\n  - metric context key must not be empty")
		}
		if !validMetricNamePattern.MatchString(k) {
			return fmt.Errorf("config validation failed:\n  - metric context key %q contains invalid characters", k)
		}
		switch v.(type) {
		case string:
			// ok
		case nil:
			return fmt.Errorf("config validation failed:\n  - metric context key %q has nil value", k)
		default:
			_, isMap := v.(map[string]any)
			_, isSlice := v.([]any)
			if isMap || isSlice {
				return fmt.Errorf("config validation failed:\n  - metric context key %q must be a scalar value, got nested structure", k)
			}
			// Other scalar types (int, float, bool) are OK — convert to string at use time.
		}
	}
	return nil
}

// ResolveUserMetricContext converts validated context values to string map.
func ResolveUserMetricContext(ctx map[string]any) map[string]string {
	if len(ctx) == 0 {
		return nil
	}
	result := make(map[string]string, len(ctx))
	for k, v := range ctx {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

