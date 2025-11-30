package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigAnalysis_SimpleFields(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package testplugin

import "github.com/sflowg/sflowg/runtime/plugin"

type Config struct {
	Host string
	Port int
	Enabled bool
}

type TestPlugin struct {
	config Config
}

func (p *TestPlugin) Task(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/plugin", "test", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	if !metadata.HasConfig {
		t.Fatal("Expected HasConfig=true")
	}

	if metadata.ConfigType == nil {
		t.Fatal("Expected ConfigType to be non-nil")
	}

	if len(metadata.ConfigType.Fields) != 3 {
		t.Fatalf("Expected 3 config fields, got %d", len(metadata.ConfigType.Fields))
	}

	// Verify field names and types
	fields := metadata.ConfigType.Fields

	if fields[0].Name != "Host" || fields[0].Type != "string" {
		t.Errorf("Expected field Host (string), got %s (%s)", fields[0].Name, fields[0].Type)
	}

	if fields[1].Name != "Port" || fields[1].Type != "int" {
		t.Errorf("Expected field Port (int), got %s (%s)", fields[1].Name, fields[1].Type)
	}

	if fields[2].Name != "Enabled" || fields[2].Type != "bool" {
		t.Errorf("Expected field Enabled (bool), got %s (%s)", fields[2].Name, fields[2].Type)
	}
}

func TestConfigAnalysis_WithTags(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package redis

import (
	"time"
	"github.com/sflowg/sflowg/runtime/plugin"
)

type Config struct {
	Addr     string        ` + "`yaml:\"addr\" default:\"localhost:6379\" validate:\"required,hostname_port\"`" + `
	Password string        ` + "`yaml:\"password\"`" + `
	DB       int           ` + "`yaml:\"db\" default:\"0\" validate:\"gte=0,lte=15\"`" + `
	PoolSize int           ` + "`yaml:\"pool_size\" default:\"10\" validate:\"gte=1,lte=1000\"`" + `
	Timeout  time.Duration ` + "`yaml:\"timeout\" default:\"30s\" validate:\"gte=1s\"`" + `
}

type RedisPlugin struct {
	config Config
}

func (p *RedisPlugin) Get(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/redis", "redis", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	if !metadata.HasConfig {
		t.Fatal("Expected HasConfig=true")
	}

	fields := metadata.ConfigType.Fields

	// Check Addr field
	addrField := findField(fields, "Addr")
	if addrField == nil {
		t.Fatal("Addr field not found")
	}
	if addrField.YAMLTag != "addr" {
		t.Errorf("Expected YAMLTag='addr', got '%s'", addrField.YAMLTag)
	}
	if addrField.DefaultTag != "localhost:6379" {
		t.Errorf("Expected DefaultTag='localhost:6379', got '%s'", addrField.DefaultTag)
	}
	if addrField.ValidateTag != "required,hostname_port" {
		t.Errorf("Expected ValidateTag='required,hostname_port', got '%s'", addrField.ValidateTag)
	}
	if addrField.Type != "string" {
		t.Errorf("Expected Type='string', got '%s'", addrField.Type)
	}

	// Check Password field (no tags)
	passwordField := findField(fields, "Password")
	if passwordField == nil {
		t.Fatal("Password field not found")
	}
	if passwordField.YAMLTag != "password" {
		t.Errorf("Expected YAMLTag='password', got '%s'", passwordField.YAMLTag)
	}
	if passwordField.DefaultTag != "" {
		t.Errorf("Expected empty DefaultTag, got '%s'", passwordField.DefaultTag)
	}
	if passwordField.ValidateTag != "" {
		t.Errorf("Expected empty ValidateTag, got '%s'", passwordField.ValidateTag)
	}

	// Check DB field
	dbField := findField(fields, "DB")
	if dbField == nil {
		t.Fatal("DB field not found")
	}
	if dbField.Type != "int" {
		t.Errorf("Expected Type='int', got '%s'", dbField.Type)
	}
	if dbField.DefaultTag != "0" {
		t.Errorf("Expected DefaultTag='0', got '%s'", dbField.DefaultTag)
	}
	if dbField.ValidateTag != "gte=0,lte=15" {
		t.Errorf("Expected ValidateTag='gte=0,lte=15', got '%s'", dbField.ValidateTag)
	}

	// Check Timeout field (time.Duration)
	timeoutField := findField(fields, "Timeout")
	if timeoutField == nil {
		t.Fatal("Timeout field not found")
	}
	if timeoutField.Type != "time.Duration" {
		t.Errorf("Expected Type='time.Duration', got '%s'", timeoutField.Type)
	}
	if timeoutField.DefaultTag != "30s" {
		t.Errorf("Expected DefaultTag='30s', got '%s'", timeoutField.DefaultTag)
	}
}

func TestConfigAnalysis_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package math

import "github.com/sflowg/sflowg/runtime/plugin"

type MathPlugin struct{}

func (p *MathPlugin) Add(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/math", "math", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	if metadata.HasConfig {
		t.Error("Expected HasConfig=false for plugin without config")
	}

	if metadata.ConfigType != nil {
		t.Error("Expected ConfigType=nil for plugin without config")
	}
}

func TestConfigAnalysis_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package test

import "github.com/sflowg/sflowg/runtime/plugin"

type Config struct{}

type TestPlugin struct {
	config Config
}

func (p *TestPlugin) Task(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/plugin", "test", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	if !metadata.HasConfig {
		t.Fatal("Expected HasConfig=true")
	}

	if metadata.ConfigType == nil {
		t.Fatal("Expected ConfigType to be non-nil")
	}

	if len(metadata.ConfigType.Fields) != 0 {
		t.Errorf("Expected 0 config fields for empty struct, got %d", len(metadata.ConfigType.Fields))
	}
}

func TestConfigAnalysis_ComplexTypes(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package complex

import (
	"time"
	"net/url"
	"github.com/sflowg/sflowg/runtime/plugin"
)

type Config struct {
	Timeout    time.Duration
	URL        *url.URL
	Tags       []string
	Metadata   map[string]string
	MaxRetries *int
}

type ComplexPlugin struct {
	config Config
}

func (p *ComplexPlugin) Task(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/complex", "complex", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	if !metadata.HasConfig {
		t.Fatal("Expected HasConfig=true")
	}

	fields := metadata.ConfigType.Fields

	// Check various type representations
	timeoutField := findField(fields, "Timeout")
	if timeoutField == nil || timeoutField.Type != "time.Duration" {
		t.Errorf("Expected Timeout field with type time.Duration, got %+v", timeoutField)
	}

	urlField := findField(fields, "URL")
	if urlField == nil || urlField.Type != "*url.URL" {
		t.Errorf("Expected URL field with type *url.URL, got %+v", urlField)
	}

	tagsField := findField(fields, "Tags")
	if tagsField == nil || tagsField.Type != "[]string" {
		t.Errorf("Expected Tags field with type []string, got %+v", tagsField)
	}

	metadataField := findField(fields, "Metadata")
	if metadataField == nil || metadataField.Type != "map[string]string" {
		t.Errorf("Expected Metadata field with type map[string]string, got %+v", metadataField)
	}

	maxRetriesField := findField(fields, "MaxRetries")
	if maxRetriesField == nil || maxRetriesField.Type != "*int" {
		t.Errorf("Expected MaxRetries field with type *int, got %+v", maxRetriesField)
	}
}

func TestConfigAnalysis_YAMLTagWithOptions(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package test

import "github.com/sflowg/sflowg/runtime/plugin"

type Config struct {
	Name     string ` + "`yaml:\"name,omitempty\"`" + `
	Password string ` + "`yaml:\"password,omitempty\"`" + `
}

type TestPlugin struct {
	config Config
}

func (p *TestPlugin) Task(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/plugin", "test", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	fields := metadata.ConfigType.Fields

	// YAML tags with options should only extract the field name part
	nameField := findField(fields, "Name")
	if nameField == nil || nameField.YAMLTag != "name" {
		t.Errorf("Expected YAMLTag='name' (without options), got '%s'", nameField.YAMLTag)
	}

	passwordField := findField(fields, "Password")
	if passwordField == nil || passwordField.YAMLTag != "password" {
		t.Errorf("Expected YAMLTag='password' (without options), got '%s'", passwordField.YAMLTag)
	}
}

// Helper function to find a field by name
func findField(fields []ConfigField, name string) *ConfigField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}
