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
