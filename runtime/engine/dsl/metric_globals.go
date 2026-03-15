package dsl

import (
	"fmt"

	"github.com/BDNK1/sflowg/runtime"
)

// BuildMetricGlobals creates the `metric` module globals for DSL step evaluation.
// Follows the same pattern as BuildLogGlobals.
func BuildMetricGlobals(exec *runtime.Execution) map[string]any {
	metrics := exec.Metrics()
	logger := exec.Logger().ForUser()

	metricMethods := map[string]any{
		"counter":       makeCounterFn(exec, metrics, logger),
		"updowncounter": makeUpDownCounterFn(exec, metrics, logger),
		"histogram":     makeHistogramFn(exec, metrics, logger),
		"gauge":         makeGaugeFn(exec, metrics, logger),
	}

	// Add predeclared handles.
	for name, decl := range metrics.UserDeclarations() {
		metricMethods[name] = buildPredeclaredHandle(exec, metrics, name, decl, logger)
	}

	return map[string]any{
		"metric": metricMethods,
	}
}

// metric.counter(name, value?, labels?)
func makeCounterFn(exec *runtime.Execution, metrics *runtime.Metrics, logger runtime.Logger) func(args ...any) error {
	return func(args ...any) error {
		if len(args) == 0 {
			logger.Warn("metric.counter called with no arguments")
			return nil
		}

		name, ok := asString(args[0])
		if !ok {
			logger.Warn("metric.counter: name must be a string", "got", fmt.Sprintf("%T", args[0]))
			return nil
		}

		value := 1.0
		var labels map[string]any

		if len(args) >= 2 {
			v, ok := asFloat64(args[1])
			if !ok {
				logger.Warn("metric.counter: value must be numeric", "metric", name, "got", fmt.Sprintf("%T", args[1]))
				return nil
			}
			value = v
		}
		if len(args) >= 3 {
			labels = asLabels(args[2])
		}

		metrics.RecordUserCounter(exec, exec, name, value, labels)
		return nil
	}
}

// metric.updowncounter(name, value, labels?)
func makeUpDownCounterFn(exec *runtime.Execution, metrics *runtime.Metrics, logger runtime.Logger) func(args ...any) error {
	return func(args ...any) error {
		if len(args) < 2 {
			logger.Warn("metric.updowncounter requires name and value arguments")
			return nil
		}

		name, ok := asString(args[0])
		if !ok {
			logger.Warn("metric.updowncounter: name must be a string", "got", fmt.Sprintf("%T", args[0]))
			return nil
		}

		value, ok := asFloat64(args[1])
		if !ok {
			logger.Warn("metric.updowncounter: value must be numeric", "metric", name, "got", fmt.Sprintf("%T", args[1]))
			return nil
		}

		var labels map[string]any
		if len(args) >= 3 {
			labels = asLabels(args[2])
		}

		metrics.RecordUserUpDownCounter(exec, exec, name, value, labels)
		return nil
	}
}

// metric.histogram(name, value, labels?)
func makeHistogramFn(exec *runtime.Execution, metrics *runtime.Metrics, logger runtime.Logger) func(args ...any) error {
	return func(args ...any) error {
		if len(args) < 2 {
			logger.Warn("metric.histogram requires name and value arguments")
			return nil
		}

		name, ok := asString(args[0])
		if !ok {
			logger.Warn("metric.histogram: name must be a string", "got", fmt.Sprintf("%T", args[0]))
			return nil
		}

		value, ok := asFloat64(args[1])
		if !ok {
			logger.Warn("metric.histogram: value must be numeric", "metric", name, "got", fmt.Sprintf("%T", args[1]))
			return nil
		}

		var labels map[string]any
		if len(args) >= 3 {
			labels = asLabels(args[2])
		}

		metrics.RecordUserHistogram(exec, exec, name, value, labels)
		return nil
	}
}

// metric.gauge(name, value, labels?)
func makeGaugeFn(exec *runtime.Execution, metrics *runtime.Metrics, logger runtime.Logger) func(args ...any) error {
	return func(args ...any) error {
		if len(args) < 2 {
			logger.Warn("metric.gauge requires name and value arguments")
			return nil
		}

		name, ok := asString(args[0])
		if !ok {
			logger.Warn("metric.gauge: name must be a string", "got", fmt.Sprintf("%T", args[0]))
			return nil
		}

		value, ok := asFloat64(args[1])
		if !ok {
			logger.Warn("metric.gauge: value must be numeric", "metric", name, "got", fmt.Sprintf("%T", args[1]))
			return nil
		}

		var labels map[string]any
		if len(args) >= 3 {
			labels = asLabels(args[2])
		}

		metrics.RecordUserGauge(exec, exec, name, value, labels)
		return nil
	}
}

// buildPredeclaredHandle builds a named handle map with the appropriate method
// for the metric type. These are nested maps containing Go functions, which
// will be recursively converted to Risor modules by mapToModule.
func buildPredeclaredHandle(exec *runtime.Execution, metrics *runtime.Metrics, name string, decl runtime.UserMetricDecl, logger runtime.Logger) map[string]any {
	switch decl.Type {
	case "counter":
		return map[string]any{
			"inc": func(args ...any) error {
				value := 1.0
				var labels map[string]any
				if len(args) >= 1 {
					v, ok := asFloat64(args[0])
					if !ok {
						logger.Warn("metric handle inc: value must be numeric", "metric", name)
						return nil
					}
					value = v
				}
				if len(args) >= 2 {
					labels = asLabels(args[1])
				}
				metrics.RecordUserCounter(exec, exec, name, value, labels)
				return nil
			},
		}
	case "updowncounter":
		return map[string]any{
			"add": func(args ...any) error {
				if len(args) < 1 {
					logger.Warn("metric handle add: value required", "metric", name)
					return nil
				}
				value, ok := asFloat64(args[0])
				if !ok {
					logger.Warn("metric handle add: value must be numeric", "metric", name)
					return nil
				}
				var labels map[string]any
				if len(args) >= 2 {
					labels = asLabels(args[1])
				}
				metrics.RecordUserUpDownCounter(exec, exec, name, value, labels)
				return nil
			},
		}
	case "histogram":
		return map[string]any{
			"observe": func(args ...any) error {
				if len(args) < 1 {
					logger.Warn("metric handle observe: value required", "metric", name)
					return nil
				}
				value, ok := asFloat64(args[0])
				if !ok {
					logger.Warn("metric handle observe: value must be numeric", "metric", name)
					return nil
				}
				var labels map[string]any
				if len(args) >= 2 {
					labels = asLabels(args[1])
				}
				metrics.RecordUserHistogram(exec, exec, name, value, labels)
				return nil
			},
		}
	case "gauge":
		return map[string]any{
			"set": func(args ...any) error {
				if len(args) < 1 {
					logger.Warn("metric handle set: value required", "metric", name)
					return nil
				}
				value, ok := asFloat64(args[0])
				if !ok {
					logger.Warn("metric handle set: value must be numeric", "metric", name)
					return nil
				}
				var labels map[string]any
				if len(args) >= 2 {
					labels = asLabels(args[1])
				}
				metrics.RecordUserGauge(exec, exec, name, value, labels)
				return nil
			},
		}
	default:
		return nil
	}
}

// --- Argument conversion helpers ---

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}

func asLabels(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
