package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

var newOTLPLogHandler = func(cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
	options := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Export.Endpoint),
	}
	if cfg.Export.Insecure {
		options = append(options, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(context.Background(), options...)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	providerOptions := []sdklog.LoggerProviderOption{
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	}
	if res, err := tracingResource(cfg.Export.Attributes); err != nil {
		_ = exporter.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("build logging resource: %w", err)
	} else if res != nil {
		providerOptions = append(providerOptions, sdklog.WithResource(res))
	}

	provider := sdklog.NewLoggerProvider(providerOptions...)
	handler := otelslog.NewHandler(tracerName, otelslog.WithLoggerProvider(provider))
	return handler, provider.Shutdown, nil
}

func newObservabilityBaseHandler(w io.Writer, cfg LoggingConfig) (slog.Handler, func(context.Context) error, error) {
	modes := normalizeLogExportModes(cfg.Export.Mode)
	children := make([]slog.Handler, 0, len(modes))
	shutdowns := make([]func(context.Context) error, 0, 1)

	if containsLogExportMode(modes, logExportModeStdout) {
		children = append(children, slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	if cfg.Export.Enabled && containsLogExportMode(modes, logExportModeOTLP) {
		handler, shutdown, err := newOTLPLogHandler(cfg)
		if err != nil {
			return nil, nil, err
		}
		children = append(children, handler)
		shutdowns = append(shutdowns, shutdown)
	}

	if len(children) == 0 {
		children = append(children, slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	base := children[0]
	if len(children) > 1 {
		base = &fanoutHandler{children: children}
	}

	return base, joinShutdowns(shutdowns), nil
}

type fanoutHandler struct {
	children []slog.Handler
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, child := range h.children {
		if child.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, child := range h.children {
		if !child.Enabled(ctx, r.Level) {
			continue
		}
		if err := child.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	children := make([]slog.Handler, len(h.children))
	for i, child := range h.children {
		children[i] = child.WithAttrs(attrs)
	}
	return &fanoutHandler{children: children}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	children := make([]slog.Handler, len(h.children))
	for i, child := range h.children {
		children[i] = child.WithGroup(name)
	}
	return &fanoutHandler{children: children}
}

func joinShutdowns(shutdowns []func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		var errs []error
		for _, shutdown := range shutdowns {
			if shutdown == nil {
				continue
			}
			if err := shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}
