package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BDNK1/sflowg/cli/internal/security"
	"github.com/BDNK1/sflowg/core/bootstrap"
	"gopkg.in/yaml.v3"
)

// FlowConfig represents the flow-config.yaml structure
type FlowConfig struct {
	Name          string                 `yaml:"name"`          // Optional: defaults to directory name
	Version       string                 `yaml:"version"`       // Optional: defaults to "latest"
	Runtime       RuntimeConfig          `yaml:"runtime"`       // Optional: runtime configuration
	Observability ObservabilityConfig    `yaml:"observability"` // Optional: observability configuration
	Properties    map[string]interface{} `yaml:"properties"`    // Optional: global properties for all flows
	Plugins       []PluginConfig         `yaml:"plugins"`
}

// RuntimeConfig represents runtime configuration
type RuntimeConfig struct {
	Port     string                `yaml:"port"`              // Optional: HTTP server port, defaults to "8080"
	Version  string                `yaml:"version,omitempty"` // Optional: runtime module version, defaults to "latest"
	Engine   string                `yaml:"engine,omitempty"`  // Optional: only "dsl" is supported
	Kafka    KafkaRuntimeConfig    `yaml:"kafka,omitempty"`
	Async    AsyncRuntimeConfig    `yaml:"async,omitempty"`
	Parallel ParallelRuntimeConfig `yaml:"parallel,omitempty"`
}

type AsyncRuntimeConfig struct {
	RuntimeMaxInFlight int `yaml:"runtime_max_in_flight,omitempty"`
}

type ParallelRuntimeConfig struct {
	BlockDefaultMaxInFlight   int    `yaml:"block_default_max_in_flight,omitempty"`
	ForeachDefaultMaxInFlight int    `yaml:"foreach_default_max_in_flight,omitempty"`
	DefaultOnFailure          string `yaml:"default_on_failure,omitempty"`
}

type KafkaRuntimeConfig struct {
	Brokers map[string]KafkaBrokerConfig `yaml:"brokers,omitempty"`
}

type KafkaBrokerConfig struct {
	Brokers               []string `yaml:"brokers"`
	ClientID              string   `yaml:"client_id,omitempty"`
	NackRedeliveryDelayMS int      `yaml:"nack_redelivery_delay_ms,omitempty"`
}

type ObservabilityConfig = bootstrap.ObservabilityConfig
type LoggingConfig = bootstrap.LoggingConfig
type LogSourcesConfig = bootstrap.LogSourcesConfig
type MaskingConfig = bootstrap.MaskingConfig
type TracingConfig = bootstrap.TracingConfig

// PluginConfig represents a single plugin configuration
type PluginConfig struct {
	Source  string                 `yaml:"source"`            // Required: plugin source location
	Name    string                 `yaml:"name,omitempty"`    // Optional: auto-detected from source if not provided
	Version string                 `yaml:"version,omitempty"` // Optional: for remote modules
	Config  map[string]interface{} `yaml:"config,omitempty"`  // Optional: plugin-specific config (Phase 2)
}

// PluginType represents the type of plugin source
type PluginType int

const (
	TypeUnknown PluginType = iota
	TypeCorePlugin
	TypeLocalModule
	TypeRemoteModule
)

func (t PluginType) String() string {
	switch t {
	case TypeCorePlugin:
		return "core"
	case TypeLocalModule:
		return "local"
	case TypeRemoteModule:
		return "remote"
	default:
		return "unknown"
	}
}

// Load reads and parses flow-config.yaml from the given directory
func Load(projectDir string) (*FlowConfig, error) {
	configPath := filepath.Join(projectDir, "flow-config.yaml")

	// Security: Validate configPath is within project directory
	if err := security.ValidatePathWithinBoundary(projectDir, configPath); err != nil {
		return nil, fmt.Errorf("invalid config path: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read flow-config.yaml from %q: %w", configPath, err)
	}

	var config FlowConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse flow-config.yaml: %w", err)
	}

	// Validate
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Apply defaults
	if err := config.ApplyDefaults(projectDir); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}

	return &config, nil
}

// Validate checks that the config has all required fields
func (c *FlowConfig) Validate() error {
	for i, plugin := range c.Plugins {
		if plugin.Source == "" {
			return fmt.Errorf("plugin #%d: source field is required", i)
		}
	}

	if c.Runtime.Engine != "" && c.Runtime.Engine != "dsl" {
		return fmt.Errorf("runtime.engine must be 'dsl', got %q", c.Runtime.Engine)
	}
	if c.Runtime.Async.RuntimeMaxInFlight < 0 {
		return fmt.Errorf("runtime.async.runtime_max_in_flight must be greater than 0")
	}
	if c.Runtime.Parallel.BlockDefaultMaxInFlight < 0 {
		return fmt.Errorf("runtime.parallel.block_default_max_in_flight must be greater than or equal to 0")
	}
	if c.Runtime.Parallel.ForeachDefaultMaxInFlight < 0 {
		return fmt.Errorf("runtime.parallel.foreach_default_max_in_flight must be greater than or equal to 0")
	}
	if c.Runtime.Parallel.DefaultOnFailure != "" &&
		c.Runtime.Parallel.DefaultOnFailure != "wait_all" &&
		c.Runtime.Parallel.DefaultOnFailure != "fail_fast" {
		return fmt.Errorf("runtime.parallel.default_on_failure must be wait_all or fail_fast")
	}

	if err := bootstrap.ValidateObservabilityConfig(c.Observability); err != nil {
		return fmt.Errorf("invalid observability config: %w", err)
	}

	return nil
}

// ApplyDefaults fills in missing optional fields with defaults
func (c *FlowConfig) ApplyDefaults(projectDir string) error {
	// Default project name to directory name
	if c.Name == "" {
		// Extract directory name from path
		c.Name = getDirectoryName(projectDir)
	}

	// Default version
	if c.Version == "" {
		c.Version = "latest"
	}

	// Default runtime configuration
	if c.Runtime.Port == "" {
		c.Runtime.Port = "8080"
	}
	if c.Runtime.Version == "" {
		c.Runtime.Version = "latest"
	}
	if c.Runtime.Engine == "" {
		c.Runtime.Engine = "dsl"
	}
	if c.Runtime.Async.RuntimeMaxInFlight == 0 {
		c.Runtime.Async.RuntimeMaxInFlight = 256
	}
	if c.Runtime.Parallel.BlockDefaultMaxInFlight == 0 {
		c.Runtime.Parallel.BlockDefaultMaxInFlight = 8
	}
	if c.Runtime.Parallel.ForeachDefaultMaxInFlight == 0 {
		c.Runtime.Parallel.ForeachDefaultMaxInFlight = 8
	}
	if c.Runtime.Parallel.DefaultOnFailure == "" {
		c.Runtime.Parallel.DefaultOnFailure = "wait_all"
	}
	for name, broker := range c.Runtime.Kafka.Brokers {
		if broker.NackRedeliveryDelayMS == 0 {
			broker.NackRedeliveryDelayMS = 100
		}
		c.Runtime.Kafka.Brokers[name] = broker
	}

	if err := bootstrap.ApplyObservabilityDefaults(&c.Observability); err != nil {
		return fmt.Errorf("failed to apply observability defaults: %w", err)
	}

	// Apply defaults to each plugin
	for i := range c.Plugins {
		c.Plugins[i].ApplyDefaults()
	}

	return nil
}

// ApplyDefaults fills in missing optional fields for a plugin
func (p *PluginConfig) ApplyDefaults() {
	// Name will be inferred during type detection if not provided
	// Version defaults to latest for remote modules
	if p.Version == "" {
		p.Version = "latest"
	}
}

// getDirectoryName extracts the last component of a path
func getDirectoryName(path string) string {
	// Handle "." case
	if path == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "sflowg-app"
		}
		path = cwd
	}

	// Use filepath.Base for platform-independent path parsing
	return filepath.Base(path)
}
