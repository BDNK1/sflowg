package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BDNK1/sflowg/core"
	dslengine "github.com/BDNK1/sflowg/core/engine/dsl"
	"github.com/BDNK1/sflowg/core/observability"
)

type Container = runtime.Container
type Transport = runtime.Transport
type ObservabilityConfig = observability.Config
type LoggingConfig = observability.LoggingConfig
type LogExportConfig = observability.LogExportConfig
type LogExportModes = observability.LogExportModes
type LogSourcesConfig = observability.LogSourcesConfig
type MaskingConfig = observability.MaskingConfig
type TracingConfig = observability.TracingConfig
type MetricsConfig = observability.MetricsConfig
type HistogramBuckets = observability.HistogramBuckets
type UserMetricsConfig = observability.UserMetricsConfig
type UserMetricDecl = observability.UserMetricDecl
type UserMetricLabel = observability.UserMetricLabel

// Config contains the project-specific inputs needed to assemble and run the
// standard runtime stack.
type Config struct {
	FlowsDir         string
	FlowsSource      string
	GlobalProperties map[string]any
	Observability    ObservabilityConfig
	RegisterPlugins  func(*Container) error
	Transports       []Transport
	ValidateFlows    bool
}

// Run assembles the standard runtime stack and starts the application.
func Run(ctx context.Context, cfg Config) (err error) {
	container := runtime.NewContainer(runtime.NewLogger(nil))
	obs, err := observability.Init(cfg.Observability)
	if err != nil {
		return fmt.Errorf("initialize observability: %w", err)
	}
	container.SetObservabilityServices(obs.Logger, obs.Tracer, obs.Metrics)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := obs.Shutdown(shutdownCtx); shutdownErr != nil {
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

	app := runtime.NewApp(container, loader, evaluator, stepExecutor, stepRunner, compiler, newValueStore)
	stepExecutor.SetSubflowInvoker(app)
	if cfg.ValidateFlows {
		app.SetFlowValidator(dslengine.NewFlowValidator())
	}
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
	if err := app.RegisterTransport(runtime.NewFlowTransport()); err != nil {
		return fmt.Errorf("register flow transport: %w", err)
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Start(runCtx, cfg.FlowsDir); err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() == nil && runCtx.Err() != nil {
			return nil
		}
		return err
	}
	return nil
}

func InitializeConfig(config any, rawValues map[string]any) error {
	return runtime.InitializeConfig(config, rawValues)
}

func DefaultObservabilityConfig() ObservabilityConfig {
	return observability.DefaultConfig()
}

func ApplyObservabilityDefaults(cfg *ObservabilityConfig) error {
	return observability.ApplyDefaults(cfg)
}

func ValidateObservabilityConfig(cfg ObservabilityConfig) error {
	return observability.ValidateConfig(cfg)
}
