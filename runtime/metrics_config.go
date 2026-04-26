package runtime

import (
	"fmt"
	"regexp"
)

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

func ValidateUserMetricsConfig(cfg UserMetricsConfig) error {
	return validateUserMetricsConfig(cfg)
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
