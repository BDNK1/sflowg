package dsl

import (
	"log/slog"

	"github.com/BDNK1/sflowg/runtime"
)

func BuildLogGlobals(exec *runtime.Execution) map[string]any {
	logger := exec.Logger().ForUser()

	logMethods := map[string]any{
		"debug": makeLogFn(logger, slog.LevelDebug),
		"info":  makeLogFn(logger, slog.LevelInfo),
		"warn":  makeLogFn(logger, slog.LevelWarn),
		"error": makeLogFn(logger, slog.LevelError),
	}

	return map[string]any{
		"log": logMethods,
	}
}

func makeLogFn(logger runtime.Logger, level slog.Level) func(args ...any) error {
	return func(args ...any) error {
		if len(args) == 0 {
			return nil
		}

		message, _ := args[0].(string)
		if message == "" {
			message = "log"
		}

		var attrs []any
		if len(args) > 1 {
			data := args[1:]
			if len(data) == 1 {
				attrs = append(attrs, "data", data[0])
			} else {
				attrs = append(attrs, "data", data)
			}
		}

		logger.Log(level, message, attrs...)
		return nil
	}
}
