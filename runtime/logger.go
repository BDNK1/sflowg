package runtime

import (
	"context"
	"log/slog"
)

// Logger wraps slog.Logger and automatically supplies a context to each log call.
type Logger struct {
	base *slog.Logger
	ctx  context.Context
}

func NewLogger(base *slog.Logger) Logger {
	if base == nil {
		base = slog.Default()
	}
	return Logger{base: base, ctx: context.Background()}
}

func (l Logger) With(args ...any) Logger {
	return Logger{
		base: l.slog().With(args...),
		ctx:  l.context(),
	}
}

func (l Logger) WithContext(ctx context.Context) Logger {
	if ctx == nil {
		ctx = context.Background()
	}
	l.ctx = ctx
	return l
}

func (l Logger) Debug(msg string, args ...any) {
	l.slog().DebugContext(l.context(), msg, args...)
}

func (l Logger) Info(msg string, args ...any) {
	l.slog().InfoContext(l.context(), msg, args...)
}

func (l Logger) Warn(msg string, args ...any) {
	l.slog().WarnContext(l.context(), msg, args...)
}

func (l Logger) Error(msg string, args ...any) {
	l.slog().ErrorContext(l.context(), msg, args...)
}

func (l Logger) Log(level slog.Level, msg string, args ...any) {
	l.slog().Log(l.context(), level, msg, args...)
}

func (l Logger) Slog() *slog.Logger {
	return l.slog()
}

// ForPlugin returns a logger with source=plugin baked into the handler and
// plugin=name added as a bound attr. Uses withSource to avoid duplicate source keys.
// Preserves any context already bound to the logger.
func (l Logger) ForPlugin(name string) Logger {
	if h, ok := l.slog().Handler().(*observabilityHandler); ok {
		return Logger{base: slog.New(h.withSource("plugin")).With("plugin", name), ctx: l.ctx}
	}
	// Fallback for non-observability handlers (e.g. tests using slog.Default).
	return Logger{base: l.slog().With("source", "plugin", "plugin", name), ctx: l.ctx}
}

// ForUser returns a logger with source=user baked into the handler.
// Used for DSL log globals so user-emitted logs are filtered and labelled correctly.
// Preserves any context already bound to the logger.
func (l Logger) ForUser() Logger {
	if h, ok := l.slog().Handler().(*observabilityHandler); ok {
		return Logger{base: slog.New(h.withSource("user")), ctx: l.ctx}
	}
	return Logger{base: l.slog().With("source", "user"), ctx: l.ctx}
}

func (l Logger) slog() *slog.Logger {
	if l.base == nil {
		return slog.Default()
	}
	return l.base
}

func (l Logger) context() context.Context {
	if l.ctx == nil {
		return context.Background()
	}
	return l.ctx
}
