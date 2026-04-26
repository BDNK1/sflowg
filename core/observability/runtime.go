package observability

import (
	"context"
	"errors"

	"github.com/BDNK1/sflowg/core"
	"go.opentelemetry.io/otel/trace"
)

var (
	initObservabilityLogger = InitLogger
	initTracingRuntime      = InitTracing
	initMetricsRuntime      = runtime.InitMetrics
)

// Runtime owns the app-scoped observability primitives and their shutdown.
type Runtime struct {
	Logger   runtime.Logger
	Tracer   trace.Tracer
	Metrics  *runtime.Metrics
	shutdown func(context.Context) error
}

func (o *Runtime) Shutdown(ctx context.Context) error {
	if o == nil || o.shutdown == nil {
		return nil
	}
	return o.shutdown(ctx)
}

func Init(cfg Config) (*Runtime, error) {
	baseLogger, shutdownLogging, err := initObservabilityLogger(cfg)
	if err != nil {
		return nil, err
	}

	tracer, shutdownTracing, err := initTracingRuntime(cfg.Tracing)
	if err != nil {
		cleanupErr := shutdownLogging(context.Background())
		if cleanupErr != nil {
			return nil, errors.Join(err, cleanupErr)
		}
		return nil, err
	}

	metrics, shutdownMetrics, err := initMetricsRuntime(cfg.Metrics)
	if err != nil {
		cleanupErr := errors.Join(
			shutdownTracing(context.Background()),
			shutdownLogging(context.Background()),
		)
		if cleanupErr != nil {
			return nil, errors.Join(err, cleanupErr)
		}
		return nil, err
	}

	return &Runtime{
		Logger:  runtime.NewLogger(baseLogger),
		Tracer:  tracer,
		Metrics: metrics,
		shutdown: joinShutdowns([]func(context.Context) error{
			shutdownTracing,
			shutdownMetrics,
			shutdownLogging,
		}),
	}, nil
}
