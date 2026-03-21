package runtime

import (
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const (
	userMetricPrefix   = "sflowg.user."
	maxLabelKeyLen     = 64
	maxLabelValueLen   = 256
	maxLabelsPerMetric = 10
)

// userMetricsState holds the user metrics registries and config.
// Stored as the named field Metrics.user to make the boundary with platform metrics explicit.
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
	if m.user.userDecls == nil {
		return nil
	}
	return m.user.userDecls
}

// SetUserMetricContext configures the property-based auto-attached context.
func (m *Metrics) SetUserMetricContext(ctx map[string]string) {
	m.user.userMetricContext = ctx
}

// InitUserMetrics creates predeclared instruments from config declarations.
// Must be called during startup after the meter is available.
func (m *Metrics) InitUserMetrics(decls map[string]UserMetricDecl) error {
	if m.user.meter == nil {
		return nil
	}
	m.user.userDecls = decls
	m.user.predeclaredCounters = make(map[string]otelmetric.Float64Counter)
	m.user.predeclaredUpDownCounters = make(map[string]otelmetric.Float64UpDownCounter)
	m.user.predeclaredHistograms = make(map[string]otelmetric.Float64Histogram)
	m.user.predeclaredGauges = make(map[string]otelmetric.Float64Gauge)

	for name, decl := range decls {
		if err := m.createPredeclaredInstrument(name, decl); err != nil {
			return fmt.Errorf("create predeclared metric %q: %w", name, err)
		}
	}
	return nil
}

func (m *Metrics) createPredeclaredInstrument(name string, decl UserMetricDecl) error {
	otelName := userMetricPrefix + name

	switch decl.Type {
	case "counter":
		inst, err := m.newCounterInstrument(otelName, decl.Description, decl.Unit)
		if err != nil {
			return err
		}
		m.user.predeclaredCounters[name] = inst
	case "updowncounter":
		inst, err := m.newUpDownCounterInstrument(otelName, decl.Description, decl.Unit)
		if err != nil {
			return err
		}
		m.user.predeclaredUpDownCounters[name] = inst
	case "histogram":
		inst, err := m.newHistogramInstrument(otelName, decl.Description, decl.Unit)
		if err != nil {
			return err
		}
		m.user.predeclaredHistograms[name] = inst
	case "gauge":
		inst, err := m.newGaugeInstrument(otelName, decl.Description, decl.Unit)
		if err != nil {
			return err
		}
		m.user.predeclaredGauges[name] = inst
	default:
		return fmt.Errorf("unknown metric type %q", decl.Type)
	}
	return nil
}

func (m *Metrics) newCounterInstrument(name, description, unit string) (otelmetric.Float64Counter, error) {
	opts := []otelmetric.Float64CounterOption{}
	if description != "" {
		opts = append(opts, otelmetric.WithDescription(description))
	}
	if unit != "" {
		opts = append(opts, otelmetric.WithUnit(unit))
	}
	return m.user.meter.Float64Counter(name, opts...)
}

func (m *Metrics) newUpDownCounterInstrument(name, description, unit string) (otelmetric.Float64UpDownCounter, error) {
	opts := []otelmetric.Float64UpDownCounterOption{}
	if description != "" {
		opts = append(opts, otelmetric.WithDescription(description))
	}
	if unit != "" {
		opts = append(opts, otelmetric.WithUnit(unit))
	}
	return m.user.meter.Float64UpDownCounter(name, opts...)
}

func (m *Metrics) newHistogramInstrument(name, description, unit string) (otelmetric.Float64Histogram, error) {
	opts := []otelmetric.Float64HistogramOption{}
	if description != "" {
		opts = append(opts, otelmetric.WithDescription(description))
	}
	if unit != "" {
		opts = append(opts, otelmetric.WithUnit(unit))
	}
	return m.user.meter.Float64Histogram(name, opts...)
}

func (m *Metrics) newGaugeInstrument(name, description, unit string) (otelmetric.Float64Gauge, error) {
	opts := []otelmetric.Float64GaugeOption{}
	if description != "" {
		opts = append(opts, otelmetric.WithDescription(description))
	}
	if unit != "" {
		opts = append(opts, otelmetric.WithUnit(unit))
	}
	return m.user.meter.Float64Gauge(name, opts...)
}

// RecordUserCounter records a user-defined counter metric.
func (m *Metrics) RecordUserCounter(exec *Execution, name string, value float64, labels map[string]any) {
	if m.user.meter == nil {
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

	ok = m.resolvePredeclaredMetricType(name, "counter", logger)
	if !ok {
		return
	}

	// Check predeclared first.
	if inst, found := m.user.predeclaredCounters[name]; found {
		inst.Add(exec, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst, ok := m.getOrCreateDynamicCounter(name, logger)
	if !ok {
		return
	}
	inst.Add(exec, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserUpDownCounter records a user-defined up-down counter metric.
func (m *Metrics) RecordUserUpDownCounter(exec *Execution, name string, value float64, labels map[string]any) {
	if m.user.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "updowncounter", labels, logger)
	if !ok {
		return
	}

	ok = m.resolvePredeclaredMetricType(name, "updowncounter", logger)
	if !ok {
		return
	}

	if inst, found := m.user.predeclaredUpDownCounters[name]; found {
		inst.Add(exec, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst, ok := m.getOrCreateDynamicUpDownCounter(name, logger)
	if !ok {
		return
	}
	inst.Add(exec, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserHistogram records a user-defined histogram metric.
func (m *Metrics) RecordUserHistogram(exec *Execution, name string, value float64, labels map[string]any) {
	if m.user.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "histogram", labels, logger)
	if !ok {
		return
	}

	ok = m.resolvePredeclaredMetricType(name, "histogram", logger)
	if !ok {
		return
	}

	if inst, found := m.user.predeclaredHistograms[name]; found {
		inst.Record(exec, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst, ok := m.getOrCreateDynamicHistogram(name, logger)
	if !ok {
		return
	}
	inst.Record(exec, value, otelmetric.WithAttributes(attrs...))
}

// RecordUserGauge records a user-defined gauge metric.
func (m *Metrics) RecordUserGauge(exec *Execution, name string, value float64, labels map[string]any) {
	if m.user.meter == nil {
		return
	}
	logger := userMetricLogger(exec)
	attrs, ok := m.buildUserMetricAttrs(exec, name, "gauge", labels, logger)
	if !ok {
		return
	}

	ok = m.resolvePredeclaredMetricType(name, "gauge", logger)
	if !ok {
		return
	}

	if inst, found := m.user.predeclaredGauges[name]; found {
		inst.Record(exec, value, otelmetric.WithAttributes(attrs...))
		return
	}

	inst, ok := m.getOrCreateDynamicGauge(name, logger)
	if !ok {
		return
	}
	inst.Record(exec, value, otelmetric.WithAttributes(attrs...))
}

// --- Dynamic instrument creation (sync.Map) ---

func (m *Metrics) getOrCreateDynamicCounter(name string, logger Logger) (otelmetric.Float64Counter, bool) {
	if existing, ok := m.user.dynamicCounters.Load(name); ok {
		return existing.(otelmetric.Float64Counter), true
	}
	if !m.checkDynamicTypeConflict(name, "counter", logger) {
		return nil, false
	}
	inst, err := m.newCounterInstrument(userMetricPrefix+name, "", "")
	if err != nil {
		logger.Warn("failed to create dynamic counter", "metric", name, "error", err)
		return nil, false
	}
	actual, _ := m.user.dynamicCounters.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Counter), true
}

func (m *Metrics) getOrCreateDynamicUpDownCounter(name string, logger Logger) (otelmetric.Float64UpDownCounter, bool) {
	if existing, ok := m.user.dynamicUpDownCounters.Load(name); ok {
		return existing.(otelmetric.Float64UpDownCounter), true
	}
	if !m.checkDynamicTypeConflict(name, "updowncounter", logger) {
		return nil, false
	}
	inst, err := m.newUpDownCounterInstrument(userMetricPrefix+name, "", "")
	if err != nil {
		logger.Warn("failed to create dynamic updowncounter", "metric", name, "error", err)
		return nil, false
	}
	actual, _ := m.user.dynamicUpDownCounters.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64UpDownCounter), true
}

func (m *Metrics) getOrCreateDynamicHistogram(name string, logger Logger) (otelmetric.Float64Histogram, bool) {
	if existing, ok := m.user.dynamicHistograms.Load(name); ok {
		return existing.(otelmetric.Float64Histogram), true
	}
	if !m.checkDynamicTypeConflict(name, "histogram", logger) {
		return nil, false
	}
	inst, err := m.newHistogramInstrument(userMetricPrefix+name, "", "")
	if err != nil {
		logger.Warn("failed to create dynamic histogram", "metric", name, "error", err)
		return nil, false
	}
	actual, _ := m.user.dynamicHistograms.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Histogram), true
}

func (m *Metrics) getOrCreateDynamicGauge(name string, logger Logger) (otelmetric.Float64Gauge, bool) {
	if existing, ok := m.user.dynamicGauges.Load(name); ok {
		return existing.(otelmetric.Float64Gauge), true
	}
	if !m.checkDynamicTypeConflict(name, "gauge", logger) {
		return nil, false
	}
	inst, err := m.newGaugeInstrument(userMetricPrefix+name, "", "")
	if err != nil {
		logger.Warn("failed to create dynamic gauge", "metric", name, "error", err)
		return nil, false
	}
	actual, _ := m.user.dynamicGauges.LoadOrStore(name, inst)
	return actual.(otelmetric.Float64Gauge), true
}

func (m *Metrics) checkDynamicTypeConflict(name, metricType string, logger Logger) bool {
	descriptor := metricType
	existing, loaded := m.user.dynamicTypes.LoadOrStore(name, descriptor)
	if loaded && existing.(string) != descriptor {
		logger.Warn("dropping metric: type conflict",
			"metric", name,
			"existing_type", existing.(string),
			"requested_type", metricType)
		return false
	}
	return true
}

func (m *Metrics) resolvePredeclaredMetricType(name, metricType string, logger Logger) bool {
	if decl, ok := m.user.userDecls[name]; ok {
		if decl.Type != metricType {
			logger.Warn("dropping metric: type does not match predeclared declaration",
				"metric", name,
				"existing_type", decl.Type,
				"requested_type", metricType)
			return false
		}
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
	for k, v := range m.user.userMetricContext {
		attrs = append(attrs, attribute.String(k, v))
	}

	// Build reserved key set including property-based context keys.
	reserved := make(map[string]bool, len(reservedUserMetricKeys)+len(m.user.userMetricContext))
	for k := range reservedUserMetricKeys {
		reserved[k] = true
	}
	for k := range m.user.userMetricContext {
		reserved[k] = true
	}

	// Validate and add user-provided labels.
	decl, isPredeclared := m.user.userDecls[name]
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
