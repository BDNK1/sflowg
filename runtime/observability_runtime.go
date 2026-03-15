package runtime

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/trace"
)

var (
	initObservabilityLogger = InitObservabilityLogger
	initTracingRuntime      = InitTracing
	initMetricsRuntime      = InitMetrics
)

// ObservabilityRuntime owns the app-scoped observability primitives and their shutdown.
type ObservabilityRuntime struct {
	Logger   Logger
	Tracer   trace.Tracer
	Metrics  *Metrics
	shutdown func(context.Context) error
}

func (o *ObservabilityRuntime) Shutdown(ctx context.Context) error {
	if o == nil || o.shutdown == nil {
		return nil
	}
	return o.shutdown(ctx)
}

func InitObservability(cfg ObservabilityConfig) (*ObservabilityRuntime, error) {
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

	return &ObservabilityRuntime{
		Logger:  NewLogger(baseLogger),
		Tracer:  tracer,
		Metrics: metrics,
		shutdown: joinShutdowns([]func(context.Context) error{
			shutdownTracing,
			shutdownMetrics,
			shutdownLogging,
		}),
	}, nil
}
