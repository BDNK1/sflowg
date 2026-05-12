package config

import (
	"strings"
	"testing"
)

func TestFlowConfigApplyDefaults_UsesRuntimeObservabilityDefaults(t *testing.T) {
	cfg := FlowConfig{
		Plugins: []PluginConfig{{Source: "core://http"}},
	}

	if err := cfg.ApplyDefaults("/tmp/demo"); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	if cfg.Observability.Logging.Level != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.Observability.Logging.Level)
	}
	if cfg.Observability.Logging.Format != "json" {
		t.Fatalf("expected default log format json, got %q", cfg.Observability.Logging.Format)
	}
	if cfg.Observability.Logging.MaxPayloadBytes != 10*1024 {
		t.Fatalf("expected default max payload bytes 10240, got %d", cfg.Observability.Logging.MaxPayloadBytes)
	}
	if cfg.Observability.Logging.Masking.Placeholder != "***" {
		t.Fatalf("expected default masking placeholder, got %q", cfg.Observability.Logging.Masking.Placeholder)
	}
	if cfg.Runtime.Engine != "dsl" {
		t.Fatalf("expected default runtime engine dsl, got %q", cfg.Runtime.Engine)
	}
	if cfg.Runtime.Parallel.BlockDefaultMaxInFlight != 8 {
		t.Fatalf("expected default parallel max in flight 8, got %d", cfg.Runtime.Parallel.BlockDefaultMaxInFlight)
	}
	if cfg.Runtime.Parallel.ForeachDefaultMaxInFlight != 8 {
		t.Fatalf("expected default foreach max in flight 8, got %d", cfg.Runtime.Parallel.ForeachDefaultMaxInFlight)
	}
	if cfg.Runtime.Parallel.DefaultOnFailure != "wait_all" {
		t.Fatalf("expected default parallel on failure wait_all, got %q", cfg.Runtime.Parallel.DefaultOnFailure)
	}
}

func TestFlowConfigValidate_RejectsInvalidForeachParallelConfig(t *testing.T) {
	cfg := FlowConfig{
		Runtime: RuntimeConfig{
			Parallel: ParallelRuntimeConfig{ForeachDefaultMaxInFlight: -1},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected foreach parallel validation error")
	}
	if !strings.Contains(err.Error(), "runtime.parallel.foreach_default_max_in_flight") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowConfigValidate_UsesRuntimeObservabilityValidation(t *testing.T) {
	cfg := FlowConfig{
		Plugins: []PluginConfig{{Source: "core://http"}},
		Observability: ObservabilityConfig{
			Logging: LoggingConfig{
				Sources: LogSourcesConfig{
					Plugin: "verbose",
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected observability validation error")
	}
	if !strings.Contains(err.Error(), "invalid observability config") {
		t.Fatalf("expected observability validation error, got %v", err)
	}
}

func TestFlowConfigValidate_RejectsRemovedYAMLEngine(t *testing.T) {
	cfg := FlowConfig{
		Plugins: []PluginConfig{{Source: "core://http"}},
		Runtime: RuntimeConfig{Engine: "yaml"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected runtime.engine validation error")
	}
	if !strings.Contains(err.Error(), "runtime.engine must be 'dsl'") {
		t.Fatalf("expected dsl-only engine validation, got %v", err)
	}
}

func TestFlowConfigValidate_AllowsNoPlugins(t *testing.T) {
	cfg := FlowConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed without plugins: %v", err)
	}
}

func TestFlowConfigValidate_RejectsInvalidParallelConfig(t *testing.T) {
	cfg := FlowConfig{
		Runtime: RuntimeConfig{
			Parallel: ParallelRuntimeConfig{DefaultOnFailure: "bad"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected parallel validation error")
	}
	if !strings.Contains(err.Error(), "runtime.parallel.default_on_failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowConfigApplyDefaults_KafkaNackRedeliveryDelay(t *testing.T) {
	cfg := FlowConfig{
		Runtime: RuntimeConfig{
			Kafka: KafkaRuntimeConfig{Brokers: map[string]KafkaBrokerConfig{
				"default": {Brokers: []string{"localhost:9092"}},
			}},
		},
	}
	if err := cfg.ApplyDefaults("/tmp/demo"); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}
	if got := cfg.Runtime.Kafka.Brokers["default"].NackRedeliveryDelayMS; got != 100 {
		t.Fatalf("NackRedeliveryDelayMS = %d, want 100", got)
	}
}
