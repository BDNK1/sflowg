package generator

const mainGoTemplate = `package main

import (
	"bufio"
	"context"
{{- if .EmbedFlows}}
	"embed"
{{- end}}
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"{{.RuntimeModulePath}}"
{{- if eq .Engine "dsl"}}
	dslengine "{{.RuntimeModulePath}}/engine/dsl"
{{- else}}
	yamlengine "{{.RuntimeModulePath}}/engine/yaml"
{{- end}}
{{- range .Plugins}}
{{- if eq .Type 3}}
	"{{$.ModuleName}}/vendored"
{{- else}}
	{{sanitize .Name}}plugin "{{.ModulePath}}"
{{- end}}
{{- end}}
)

{{- if .EmbedFlows}}

//go:embed all:flows
var flowsFS embed.FS
{{- end}}

func main() {
	// Load .env file if present (before parsing flags)
	loadEnvFile()

	// Parse command-line flags
	flowsPath := flag.String("flows", "", "Path to flows directory (default: auto-detect)")
	port := flag.String("port", "{{.Port}}", "Server port")
	flag.Parse()

	ctx := context.Background()

	// Create container
	container := runtime.NewContainer()

	// Initialize plugins in dependency order (dependencies first)
	// Phase 2.2: Automatic dependency injection via struct fields
	// Phase 2.3: Configuration system with env vars and validation
{{- range $plugin := .Plugins}}

	// ===== {{$plugin.Name}} Plugin =====
{{- if $plugin.HasConfig}}
	// Initialize config (defaults → env vars → literals → validation)
{{- if eq $plugin.Type 3}}
	{{sanitize $plugin.Name}}Config := vendored.Config{}
{{- else}}
	{{sanitize $plugin.Name}}Config := {{sanitize $plugin.Name}}plugin.Config{}
{{- end}}
	{{sanitize $plugin.Name}}RawValues := make(map[string]any)
{{- if $plugin.ConfigGen}}
{{- if $plugin.ConfigGen.EnvVars}}

	// Environment variable overrides
{{- range $plugin.ConfigGen.EnvVars}}
	if val := os.Getenv("{{.EnvVar}}"); val != "" {
		{{sanitize $plugin.Name}}RawValues["{{.YAMLField}}"] = val
{{- if .Required}}
	} else {
		panic("Required environment variable {{.EnvVar}} not set")
{{- else}}
	} else {
		// Use default value from flow-config.yaml
		{{sanitize $plugin.Name}}RawValues["{{.YAMLField}}"] = "{{.DefaultValue}}"
{{- end}}
	}
{{- end}}
{{- end}}

{{- if $plugin.ConfigGen.Literals}}
	// Literal values from flow-config.yaml
{{- range $plugin.ConfigGen.Literals}}
	{{sanitize $plugin.Name}}RawValues["{{.YAMLField}}"] = {{.Value}}
{{- end}}
{{- end}}
{{- end}}

	// DEBUG: Print rawValues before InitializeConfig
	fmt.Printf("[config] DEBUG {{$plugin.Name}}: rawValues = %+v\n", {{sanitize $plugin.Name}}RawValues)

	// Apply defaults, merge values, and validate
	if err := runtime.InitializeConfig(&{{sanitize $plugin.Name}}Config, {{sanitize $plugin.Name}}RawValues); err != nil {
		panic(fmt.Sprintf("Failed to initialize {{$plugin.Name}} config: %v", err))
	}

	// DEBUG: Print config after InitializeConfig
	fmt.Printf("[config] DEBUG {{$plugin.Name}}: config = %+v\n", {{sanitize $plugin.Name}}Config)
{{- end}}

	// Create plugin instance
{{- if eq $plugin.Type 3}}
	{{sanitize $plugin.Name}}Plugin := &vendored.{{$plugin.TypeName}}{
{{- if $plugin.HasConfig}}
		Config: {{sanitize $plugin.Name}}Config,
{{- end}}
{{- range $plugin.Dependencies}}
		{{.FieldName}}: {{sanitize .PluginName}}Plugin,
{{- end}}
	}
{{- else}}
	{{sanitize $plugin.Name}}Plugin := &{{sanitize $plugin.Name}}plugin.{{$plugin.TypeName}}{
{{- if $plugin.HasConfig}}
		Config: {{sanitize $plugin.Name}}Config,
{{- end}}
{{- range $plugin.Dependencies}}
		{{.FieldName}}: {{sanitize .PluginName}}Plugin,
{{- end}}
	}
{{- end}}

	// Register plugin
	if err := container.RegisterPlugin("{{$plugin.Name}}", {{sanitize $plugin.Name}}Plugin); err != nil {
		panic(fmt.Sprintf("Failed to register plugin '{{$plugin.Name}}': %v", err))
	}
{{- end}}

	// Determine flows directory
{{- if .EmbedFlows}}
	// Embedded mode: extract to temp directory
	flowsDir, err := extractEmbeddedFlows(flowsFS, "flows")
	if err != nil {
		panic(fmt.Sprintf("Failed to extract embedded flows: %v", err))
	}
	defer os.RemoveAll(flowsDir) // Cleanup temp directory on exit
{{- else}}
	// Runtime mode: detect flows location at startup
	flowsDir, err := findFlowsPath(*flowsPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to locate flows directory: %v\nUse --flows flag to specify location explicitly", err))
	}
{{- end}}

	// Create engine components
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
{{- if eq .Engine "dsl"}}
	loader := dslengine.NewFlowLoader()
	evaluator := dslengine.NewExpressionEvaluator()
	stepExecutor := dslengine.NewStepExecutor(logger)
	newValueStore := func() runtime.ValueStore { return dslengine.NewValueStore() }
{{- else}}
	loader := yamlengine.NewFlowLoader()
	evaluator := yamlengine.NewExpressionEvaluator()
	stepExecutor := yamlengine.NewStepExecutor(evaluator, logger)
	newValueStore := func() runtime.ValueStore { return yamlengine.NewValueStore() }
{{- end}}

	// Create app and start server (runtime handles everything)
	// Runtime will: Initialize plugins → LoadFlows → Setup Gin → Handle signals → Graceful shutdown
	app := runtime.NewApp(container, loader, evaluator, stepExecutor, newValueStore)

{{- if .GlobalProperties}}
	// Set global properties from flow-config.yaml
	globalProperties := map[string]any{
{{- range $key, $value := .GlobalProperties}}
		"{{$key}}": {{printf "%#v" $value}},
{{- end}}
	}
	app.SetGlobalProperties(globalProperties)
{{- end}}

	if err := app.Start(ctx, ":"+*port, flowsDir); err != nil {
		panic(fmt.Sprintf("Server error: %v", err))
	}
}

{{- if .EmbedFlows}}

// extractEmbeddedFlows extracts embedded flows to a temporary directory
func extractEmbeddedFlows(fsys embed.FS, dir string) (string, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "sflowg-flows-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Read embedded directory
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to read embedded directory: %w", err)
	}

	// Extract each .yaml file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if it's a flow file (YAML or DSL)
		matchedYAML, _ := filepath.Match("*.yaml", entry.Name())
		matchedDSL, _ := filepath.Match("*.flow", entry.Name())
		if !matchedYAML && !matchedDSL {
			continue
		}

		// Read embedded file
		embeddedPath := filepath.Join(dir, entry.Name())
		data, err := fsys.ReadFile(embeddedPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to read embedded file %s: %w", embeddedPath, err)
		}

		// Write to temp directory
		tempFilePath := filepath.Join(tempDir, entry.Name())
		if err := os.WriteFile(tempFilePath, data, 0644); err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("failed to write file %s: %w", tempFilePath, err)
		}
	}

	return tempDir, nil
}
{{- else}}

// findFlowsPath locates the flows directory using smart detection
func findFlowsPath(flagPath string) (string, error) {
	// Priority 1: Use flag if provided
	if flagPath != "" {
		if _, err := os.Stat(flagPath); err == nil {
			return flagPath, nil
		}
		return "", fmt.Errorf("flows path from --flows flag does not exist: %s", flagPath)
	}

	// Priority 2: Environment variable
	if envPath := os.Getenv("FLOWS_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
		return "", fmt.Errorf("FLOWS_PATH environment variable points to non-existent path: %s", envPath)
	}

	// Priority 3: Relative to binary location (flows/ subdirectory)
	exe, err := os.Executable()
	if err == nil {
		binaryDir := filepath.Dir(exe)
		flowsPath := filepath.Join(binaryDir, "flows")
		if _, err := os.Stat(flowsPath); err == nil {
			return flowsPath, nil
		}

		// Priority 4: Same directory as binary
		if _, err := os.Stat(binaryDir); err == nil {
			return binaryDir, nil
		}
	}

	return "", fmt.Errorf("flows directory not found at expected location\n\nOptions:\n  1. Place flows/ directory next to binary\n  2. Set FLOWS_PATH environment variable\n  3. Use --flows flag\n  4. Place flow files in same directory as binary\n\nExample:\n  FLOWS_PATH=/data/flows ./myapp\n  ./myapp --flows /data/flows")
}
{{- end}}

// loadEnvFile loads environment variables from .env file next to the binary
// Only sets variables that are not already set in the environment
func loadEnvFile() {
	// Find .env file next to binary
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("[env] DEBUG: Failed to get executable path: %v\n", err)
		return
	}

	fmt.Printf("[env] DEBUG: Executable path: %s\n", exe)
	envPath := filepath.Join(filepath.Dir(exe), ".env")
	fmt.Printf("[env] DEBUG: Looking for .env at: %s\n", envPath)

	file, err := os.Open(envPath)
	if err != nil {
		fmt.Printf("[env] DEBUG: Failed to open .env: %v\n", err)
		return
	}
	defer file.Close()
	fmt.Printf("[env] DEBUG: Successfully opened .env file\n")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			fmt.Printf("[env] DEBUG: Skipping malformed line: %q\n", line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Only set if not already in environment
		if os.Getenv(key) == "" {
			fmt.Printf("[env] DEBUG: Setting %s=%s\n", key, value)
			os.Setenv(key, value)
		} else {
			fmt.Printf("[env] DEBUG: Skipping %s (already set)\n", key)
		}
	}
}
`
