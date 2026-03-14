package dsl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/runtime"
)

func TestBuildLogGlobals_UsesExecutionContext(t *testing.T) {
	var buf bytes.Buffer
	logger := runtime.NewObservabilityLoggerWithWriter(&buf, runtime.ObservabilityConfig{
		Logging: runtime.LoggingConfig{
			Level:           "debug",
			MaxPayloadBytes: 10240,
		},
	})

	container := runtime.NewContainer(runtime.NewLogger(logger))

	exec := &runtime.Execution{
		ID:        "exec-123",
		Flow:      &runtime.Flow{ID: "payment_flow"},
		Container: container,
	}

	globals := BuildLogGlobals(exec)
	logModule := globals["log"].(map[string]any)
	info := logModule["info"].(func(args ...any) error)

	exec.WithActiveStep("charge_card", func() {
		if err := info("payment started", map[string]any{"status": "pending"}); err != nil {
			t.Fatalf("info() returned error: %v", err)
		}
	})

	output := buf.String()
	for _, want := range []string{`"source":"user"`, `"execution_id":"exec-123"`, `"flow_id":"payment_flow"`, `"step_id":"charge_card"`, `"payment started"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected log output to contain %s, got %s", want, output)
		}
	}
}

func TestBuildLogGlobals_PreservesExtraArguments(t *testing.T) {
	var buf bytes.Buffer
	logger := runtime.NewObservabilityLoggerWithWriter(&buf, runtime.ObservabilityConfig{
		Logging: runtime.LoggingConfig{
			Level: "debug",
		},
	})

	container := runtime.NewContainer(runtime.NewLogger(logger))

	exec := &runtime.Execution{
		ID:        "exec-456",
		Flow:      &runtime.Flow{ID: "payment_flow"},
		Container: container,
	}

	globals := BuildLogGlobals(exec)
	logModule := globals["log"].(map[string]any)
	info := logModule["info"].(func(args ...any) error)

	if err := info("payment started", "a", "b", "c"); err != nil {
		t.Fatalf("info() returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"data":["a","b","c"]`) {
		t.Fatalf("expected all extra arguments to be preserved, got %s", output)
	}
}
