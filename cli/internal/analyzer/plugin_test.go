package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzePlugin_SimpleDependency(t *testing.T) {
	// Create temporary directory with test plugin
	tmpDir := t.TempDir()

	pluginCode := `package testplugin

import (
	"context"
	"github.com/sflowg/sflowg/runtime/plugin"
	"github.com/sflowg/sflowg/plugins/http"
)

type Config struct {
	Timeout int ` + "`yaml:\"timeout\" default:\"30\"`" + `
}

type TestPlugin struct {
	config Config
	http   *http.HTTPPlugin
}

func (p *TestPlugin) Initialize(ctx context.Context) error {
	return nil
}

func (p *TestPlugin) DoSomething(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return map[string]any{"result": "ok"}, nil
}
`

	// Write test plugin file
	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Analyze plugin
	metadata, err := AnalyzePlugin("test/plugin", "test", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	// Verify plugin metadata
	if metadata.Name != "test" {
		t.Errorf("Expected Name='test', got '%s'", metadata.Name)
	}

	if metadata.TypeName != "TestPlugin" {
		t.Errorf("Expected TypeName='TestPlugin', got '%s'", metadata.TypeName)
	}

	if !metadata.HasConfig {
		t.Error("Expected HasConfig=true")
	}

	// Verify dependency detection
	if len(metadata.Dependencies) != 1 {
		t.Fatalf("Expected 1 dependency, got %d", len(metadata.Dependencies))
	}

	dep := metadata.Dependencies[0]
	if dep.FieldName != "http" {
		t.Errorf("Expected FieldName='http', got '%s'", dep.FieldName)
	}

	if dep.PluginName != "http" {
		t.Errorf("Expected PluginName='http', got '%s'", dep.PluginName)
	}

	if dep.IsExported {
		t.Error("Expected IsExported=false for lowercase field 'http'")
	}

	// Verify task detection (should detect both Initialize and DoSomething, but only DoSomething has valid signature)
	var validTask *TaskMetadata
	for i := range metadata.Tasks {
		if metadata.Tasks[i].HasValidSignature {
			validTask = &metadata.Tasks[i]
			break
		}
	}
	if validTask == nil {
		t.Fatalf("Expected 1 valid task, got 0")
	}

	if validTask.MethodName != "DoSomething" {
		t.Errorf("Expected MethodName='DoSomething', got '%s'", validTask.MethodName)
	}

	expectedTaskName := "testplugin.doSomething"
	if validTask.TaskName != expectedTaskName {
		t.Errorf("Expected TaskName='%s', got '%s'", expectedTaskName, validTask.TaskName)
	}
}

func TestAnalyzePlugin_MultipleDependencies(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package payment

import (
	"github.com/sflowg/sflowg/runtime/plugin"
	"github.com/sflowg/sflowg/plugins/http"
	"github.com/sflowg/sflowg/plugins/redis"
)

type PaymentPlugin struct {
	config Config
	http   *http.HTTPPlugin
	redis  *redis.RedisPlugin
}

type Config struct{}

func (p *PaymentPlugin) Charge(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/payment", "payment", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	// Verify multiple dependencies detected
	if len(metadata.Dependencies) != 2 {
		t.Fatalf("Expected 2 dependencies, got %d", len(metadata.Dependencies))
	}

	// Check http dependency
	httpDep := findDependency(metadata.Dependencies, "http")
	if httpDep == nil {
		t.Fatal("http dependency not found")
	}

	if httpDep.PluginName != "http" {
		t.Errorf("Expected http PluginName='http', got '%s'", httpDep.PluginName)
	}

	// Check redis dependency
	redisDep := findDependency(metadata.Dependencies, "redis")
	if redisDep == nil {
		t.Fatal("redis dependency not found")
	}

	if redisDep.PluginName != "redis" {
		t.Errorf("Expected redis PluginName='redis', got '%s'", redisDep.PluginName)
	}
}

func TestAnalyzePlugin_InjectTag(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package app

import (
	"github.com/sflowg/sflowg/runtime/plugin"
	"github.com/sflowg/sflowg/plugins/redis"
)

type AppPlugin struct {
	cache    *redis.RedisPlugin ` + "`inject:\"redis_cache\"`" + `
	sessions *redis.RedisPlugin ` + "`inject:\"redis_sessions\"`" + `
}

func (p *AppPlugin) Process(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	return nil, nil
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	metadata, err := AnalyzePlugin("test/app", "app", tmpDir)
	if err != nil {
		t.Fatalf("AnalyzePlugin failed: %v", err)
	}

	// Verify inject tags parsed correctly
	if len(metadata.Dependencies) != 2 {
		t.Fatalf("Expected 2 dependencies, got %d", len(metadata.Dependencies))
	}

	cacheDep := findDependency(metadata.Dependencies, "cache")
	if cacheDep == nil {
		t.Fatal("cache dependency not found")
	}

	if cacheDep.PluginName != "redis_cache" {
		t.Errorf("Expected PluginName='redis_cache', got '%s'", cacheDep.PluginName)
	}

	if cacheDep.InjectTag != "redis_cache" {
		t.Errorf("Expected InjectTag='redis_cache', got '%s'", cacheDep.InjectTag)
	}

	sessionsDep := findDependency(metadata.Dependencies, "sessions")
	if sessionsDep == nil {
		t.Fatal("sessions dependency not found")
	}

	if sessionsDep.PluginName != "redis_sessions" {
		t.Errorf("Expected PluginName='redis_sessions', got '%s'", sessionsDep.PluginName)
	}
}

func TestAnalyzePlugin_IgnoresNonPluginFields(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package test

import (
	"net/http"
	"github.com/sflowg/sflowg/runtime/plugin"
)

type TestPlugin struct {
	config     Config
	httpClient *http.Client      // Not a plugin - should be ignored
	counter    int                // Not a pointer - should be ignored
	internal   *internalHelper   // Not exported - should be ignored
}

type Config struct{}

type internalHelper struct{}

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

	// Should have no dependencies (all fields ignored)
	if len(metadata.Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies, got %d: %+v", len(metadata.Dependencies), metadata.Dependencies)
	}
}

func TestAnalyzePlugin_NoDependencies(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package math

import "github.com/sflowg/sflowg/runtime/plugin"

type MathPlugin struct{}

func (p *MathPlugin) Add(exec *plugin.Execution, args map[string]any) (map[string]any, error) {
	a := args["a"].(float64)
	b := args["b"].(float64)
	return map[string]any{"result": a + b}, nil
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

	if len(metadata.Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies for stateless plugin, got %d", len(metadata.Dependencies))
	}

	if metadata.HasConfig {
		t.Error("Expected HasConfig=false for plugin without config")
	}

	// Should still detect task
	if len(metadata.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(metadata.Tasks))
	}
}

func TestAnalyzePlugin_ConfigDetection(t *testing.T) {
	tmpDir := t.TempDir()

	pluginCode := `package test

import "github.com/sflowg/sflowg/runtime/plugin"

type Config struct {
	Host string ` + "`yaml:\"host\" default:\"localhost\"`" + `
	Port int    ` + "`yaml:\"port\" default:\"8080\"`" + `
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
		t.Error("Expected HasConfig=true")
	}

	if metadata.ConfigType == nil {
		t.Fatal("Expected ConfigType to be non-nil")
	}

	if metadata.ConfigType.TypeName != "Config" {
		t.Errorf("Expected ConfigType.TypeName='Config', got '%s'", metadata.ConfigType.TypeName)
	}
}

func TestAnalyzePlugin_NoPluginStructFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Code without a *Plugin struct
	pluginCode := `package test

type Helper struct {
	data string
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "plugin.go"), []byte(pluginCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = AnalyzePlugin("test/invalid", "test", tmpDir)
	if err == nil {
		t.Fatal("Expected error for package without plugin struct")
	}

	analysisErr, ok := err.(*AnalysisError)
	if !ok {
		t.Fatalf("Expected AnalysisError, got %T", err)
	}

	if analysisErr.PluginName != "test" {
		t.Errorf("Expected PluginName='test' in error, got '%s'", analysisErr.PluginName)
	}
}

// Helper function to find dependency by field name
func findDependency(deps []Dependency, fieldName string) *Dependency {
	for i := range deps {
		if deps[i].FieldName == fieldName {
			return &deps[i]
		}
	}
	return nil
}
