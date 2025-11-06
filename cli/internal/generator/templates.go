package generator

const mainGoTemplate = `package main

import (
	"context"
{{- if .EmbedFlows}}
	"embed"
{{- end}}
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
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
	flag.Parse()

	ctx := context.Background()

	// Create container
	container := runtime.NewContainer()

	// Register plugins
{{- range .Plugins}}
{{- if eq .Type 3}}
	if err := container.RegisterPlugin("{{.Name}}", vendored.New{{capitalize .Name}}Plugin()); err != nil {
		panic(fmt.Sprintf("Failed to register plugin '{{.Name}}': %v", err))
	}
{{- else}}
	if err := container.RegisterPlugin("{{.Name}}", {{.Name}}plugin.New{{capitalize .Name}}Plugin()); err != nil {
		panic(fmt.Sprintf("Failed to register plugin '{{.Name}}': %v", err))
	}
{{- end}}
{{- end}}

	// Initialize plugins
	if err := container.Initialize(ctx); err != nil {
		panic(fmt.Sprintf("Failed to initialize container: %v", err))
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		if err := container.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
		}
		os.Exit(0)
	}()

	// Load flows
	var app *runtime.App
	var err error

{{- if .EmbedFlows}}
	// Embedded mode: always use embedded filesystem
	app, err = runtime.NewAppFromFS(flowsFS, "flows")
	if err != nil {
		panic(fmt.Sprintf("Failed to load embedded flows: %v", err))
	}
{{- else}}
	// Runtime mode: detect flows location
	detectedPath, detectErr := findFlowsPath(*flowsPath)
	if detectErr != nil {
		panic(fmt.Sprintf("Failed to locate flows directory: %v\nUse --flows flag to specify location explicitly", detectErr))
	}

	app, err = runtime.NewApp(detectedPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load flows from %q: %v", detectedPath, err))
	}
{{- end}}

	// Use our configured container
	app.Container = container

	// Create HTTP server and register flow handlers
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	g := gin.Default()
	executor := runtime.NewExecutor(logger)

	for _, flow := range app.Flows {
		if flow.Entrypoint.Type == "http" {
			flowCopy := flow
			runtime.NewHttpHandler(&flowCopy, container, executor, g)
		}
	}

	// Start HTTP server
	fmt.Println("Starting server on :{{.Port}}")
	if err := g.Run(":{{.Port}}"); err != nil {
		panic(fmt.Sprintf("Failed to start server: %v", err))
	}
}

{{- if not .EmbedFlows}}

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
