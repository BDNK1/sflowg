package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BDNK1/sflowg/cli/internal/security"
	"gopkg.in/yaml.v3"
)

// FlowConfig represents the flow-config.yaml structure
type FlowConfig struct {
	Name       string                 `yaml:"name"`       // Optional: defaults to directory name
	Version    string                 `yaml:"version"`    // Optional: defaults to "latest"
	Runtime    RuntimeConfig          `yaml:"runtime"`    // Optional: runtime configuration
	Properties map[string]interface{} `yaml:"properties"` // Optional: global properties for all flows
	Plugins    []PluginConfig         `yaml:"plugins"`
}

// RuntimeConfig represents runtime configuration
type RuntimeConfig struct {
	Port    string `yaml:"port"`              // Optional: HTTP server port, defaults to "8080"
	Version string `yaml:"version,omitempty"` // Optional: runtime module version, defaults to "latest"
}

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
	config.ApplyDefaults(projectDir)

	return &config, nil
}

// Validate checks that the config has all required fields
func (c *FlowConfig) Validate() error {
	if len(c.Plugins) == 0 {
		return fmt.Errorf("at least one plugin must be specified")
	}

	for i, plugin := range c.Plugins {
		if plugin.Source == "" {
			return fmt.Errorf("plugin #%d: source field is required", i)
		}
	}

	return nil
}

// ApplyDefaults fills in missing optional fields with defaults
func (c *FlowConfig) ApplyDefaults(projectDir string) {
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

	// Apply defaults to each plugin
	for i := range c.Plugins {
		c.Plugins[i].ApplyDefaults()
	}
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
