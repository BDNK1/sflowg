package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type App struct {
	Container        *Container
	Flows            map[string]Flow
	GlobalProperties map[string]any // Global properties from flow-config.yaml
	server           *http.Server
}

// NewApp creates a new application with the given container.
// Container should have plugins registered before calling Initialize.
// globalProperties are loaded from flow-config.yaml and merged with flow-level properties.
func NewApp(container *Container) *App {
	return &App{
		Container:        container,
		Flows:            make(map[string]Flow),
		GlobalProperties: make(map[string]any),
	}
}

// SetGlobalProperties sets global properties that will be merged with flow properties.
// Flow properties override global properties.
func (a *App) SetGlobalProperties(props map[string]any) {
	a.GlobalProperties = props
}

// Start starts the HTTP server and blocks until shutdown.
// Automatically handles: Initialize → LoadFlows → Gin setup → Signal handling → Graceful shutdown
// Port should be in format ":8080" or "0.0.0.0:8080"
func (a *App) Start(ctx context.Context, port string, flowsDir string) error {
	// Initialize plugins
	if err := a.initialize(ctx); err != nil {
		return err
	}

	// Load flows at startup (runtime resolution)
	if err := a.loadFlows(flowsDir); err != nil {
		return err
	}

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Create executor for flow execution
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	executor := NewExecutor(logger)

	// Register flow endpoints
	for flowID := range a.Flows {
		flow := a.Flows[flowID] // Copy to avoid pointer issues
		NewHttpHandler(&flow, a.Container, executor, a.GlobalProperties, router)
	}

	// Create HTTP server
	a.server = &http.Server{
		Addr:    port,
		Handler: router,
	}

	// Setup graceful shutdown
	shutdownChan := make(chan error, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")

		// Create shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown server and container
		if err := a.shutdown(shutdownCtx); err != nil {
			shutdownChan <- err
		}
		close(shutdownChan)
	}()

	// Start server
	fmt.Printf("Server listening on %s\n", port)
	fmt.Printf("Loaded %d flow(s)\n", len(a.Flows))

	err := a.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	// Wait for graceful shutdown to complete
	if shutdownErr := <-shutdownChan; shutdownErr != nil {
		return shutdownErr
	}

	return nil
}

// LoadFlows loads flow definitions from the specified directory.
// Supports both embedded flows (at build time) and runtime loading.
func (a *App) loadFlows(flowsDir string) error {
	if flowsDir == "" {
		return fmt.Errorf("flows directory not specified")
	}

	files, err := filepath.Glob(filepath.Join(flowsDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("error reading flows directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no flow files found in %s", flowsDir)
	}

	for _, file := range files {
		flow, err := readFlow(file)
		if err != nil {
			return fmt.Errorf("error loading flow from %s: %w", file, err)
		}
		a.registerFlow(flow)
	}

	return nil
}

// Initialize initializes the container (calls plugin Initialize methods).
// Must be called after plugins are registered and before LoadFlows.
func (a *App) initialize(ctx context.Context) error {
	if err := a.Container.Initialize(ctx); err != nil {
		return fmt.Errorf("container initialization failed: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server and container.
// Calls plugin Shutdown methods in reverse order of initialization.
func (a *App) shutdown(ctx context.Context) error {
	var errors []error

	// Shutdown HTTP server first
	if a.server != nil {
		fmt.Println("Shutting down HTTP server...")
		if err := a.server.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("http server shutdown: %w", err))
		}
	}

	// Shutdown container (calls plugin Shutdown methods)
	fmt.Println("Shutting down plugins...")
	if err := a.Container.Shutdown(ctx); err != nil {
		errors = append(errors, fmt.Errorf("container shutdown: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	fmt.Println("Shutdown complete")
	return nil
}

func (a *App) registerFlow(flow Flow) {
	a.Flows[flow.ID] = flow
}

func readFlow(file string) (Flow, error) {
	yamlFile, err := os.ReadFile(file)
	if err != nil {
		return Flow{}, fmt.Errorf("error reading YAML file: %w", err)
	}

	var flow Flow
	err = yaml.Unmarshal(yamlFile, &flow)
	if err != nil {
		return Flow{}, fmt.Errorf("error unmarshalling YAML: %w", err)
	}

	return flow, nil
}
