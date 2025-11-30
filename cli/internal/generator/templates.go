package generator

const mainGoTemplate = `package main

import (
	"context"
{{- if .EmbedFlows}}
	"embed"
{{- end}}
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"{{.RuntimeModulePath}}"
{{- range .Plugins}}
{{- if eq .Type 3}}
	"{{$.ModuleName}}/vendored"
{{- else}}
	{{.Name}}plugin "{{.ModulePath}}"
{{- end}}
{{- end}}
)

{{- if .EmbedFlows}}

//go:embed all:flows
var flowsFS embed.FS
{{- end}}

func main() {
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
	{{$plugin.Name}}Config := vendored.Config{}
{{- else}}
	{{$plugin.Name}}Config := {{$plugin.Name}}plugin.Config{}
{{- end}}
	{{$plugin.Name}}RawValues := make(map[string]any)
{{- if $plugin.ConfigGen}}
{{- if $plugin.ConfigGen.EnvVars}}

	// Environment variable overrides
{{- range $plugin.ConfigGen.EnvVars}}
	if val := os.Getenv("{{.EnvVar}}"); val != "" {
		{{$plugin.Name}}RawValues["{{.YAMLField}}"] = val
{{- if .Required}}
	} else {
		panic("Required environment variable {{.EnvVar}} not set")
{{- end}}
	}
{{- end}}
{{- end}}

{{- if $plugin.ConfigGen.Literals}}
	// Literal values from flow-config.yaml
{{- range $plugin.ConfigGen.Literals}}
	{{$plugin.Name}}RawValues["{{.YAMLField}}"] = {{.Value}}
{{- end}}
{{- end}}
{{- end}}

	// Apply defaults, merge values, and validate
	if err := runtime.InitializeConfig(&{{$plugin.Name}}Config, {{$plugin.Name}}RawValues); err != nil {
		panic(fmt.Sprintf("Failed to initialize {{$plugin.Name}} config: %v", err))
	}
{{- end}}

	// Create plugin instance
{{- if eq $plugin.Type 3}}
	{{$plugin.Name}}Plugin := &vendored.{{$plugin.TypeName}}{
{{- if $plugin.HasConfig}}
		Config: {{$plugin.Name}}Config,
{{- end}}
{{- range $plugin.Dependencies}}
		{{.FieldName}}: {{.PluginName}}Plugin,
{{- end}}
	}
{{- else}}
	{{$plugin.Name}}Plugin := &{{$plugin.Name}}plugin.{{$plugin.TypeName}}{
{{- if $plugin.HasConfig}}
		Config: {{$plugin.Name}}Config,
{{- end}}
{{- range $plugin.Dependencies}}
		{{.FieldName}}: {{.PluginName}}Plugin,
{{- end}}
	}
{{- end}}

	// Register plugin
	if err := container.RegisterPlugin("{{$plugin.Name}}", {{$plugin.Name}}Plugin); err != nil {
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

	// Create app and start server (runtime handles everything)
	// Runtime will: Initialize plugins → LoadFlows → Setup Gin → Handle signals → Graceful shutdown
	app := runtime.NewApp(container)

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

		// Check if it's a YAML file
		matched, err := filepath.Match("*.yaml", entry.Name())
		if err != nil || !matched {
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
`
