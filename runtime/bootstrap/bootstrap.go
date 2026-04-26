package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/BDNK1/sflowg/runtime"
	dslengine "github.com/BDNK1/sflowg/runtime/engine/dsl"
)

// Config contains the project-specific inputs needed to assemble and run the
// standard runtime stack.
type Config struct {
	FlowsDir         string
	FlowsSource      string
	GlobalProperties map[string]any
	Observability    runtime.ObservabilityConfig
	RegisterPlugins  func(*runtime.Container) error
	Transports       []runtime.Transport
}

// Run assembles the standard runtime stack and starts the application.
func Run(ctx context.Context, cfg Config) (err error) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	if err := container.InitObservability(cfg.Observability); err != nil {
		return fmt.Errorf("initialize observability: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := container.ShutdownObservability(shutdownCtx); shutdownErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; observability shutdown: %v", err, shutdownErr)
				return
			}
			err = fmt.Errorf("observability shutdown: %w", shutdownErr)
		}
	}()

	if cfg.RegisterPlugins != nil {
		if err := cfg.RegisterPlugins(container); err != nil {
			return fmt.Errorf("register plugins: %w", err)
		}
	}

	if cfg.FlowsSource != "" {
		container.Logger().Info("Resolved flows directory", "path", cfg.FlowsDir, "source", cfg.FlowsSource)
	}

	loader := dslengine.NewFlowLoader()
	evaluator := dslengine.NewExpressionEvaluator()
	stepExecutor := dslengine.NewStepExecutor()
	stepRunner := dslengine.NewLocalStepRunner(stepExecutor)
	compiler := dslengine.NewCompiler()
	newValueStore := func() runtime.ValueStore { return runtime.NewValueStore() }

	app := runtime.NewApp(container, loader, evaluator, stepExecutor, stepRunner, compiler, newValueStore, cfg.Observability)
	if len(cfg.GlobalProperties) > 0 {
		if err := app.SetGlobalProperties(cfg.GlobalProperties); err != nil {
			return fmt.Errorf("set global properties: %w", err)
		}
	}
	for _, transport := range cfg.Transports {
		if err := app.RegisterTransport(transport); err != nil {
			return fmt.Errorf("register %s transport: %w", transport.Type(), err)
		}
	}

	return app.Start(ctx, cfg.FlowsDir)
}
