package flowtransport

import (
	"context"
	"testing"

	"github.com/BDNK1/sflowg/runtime"
)

func TestTransportBasics(t *testing.T) {
	transport := New()
	if transport.Type() != "flow" {
		t.Fatalf("Type() = %q, want flow", transport.Type())
	}
	if got := transport.ResponseSubtypes(); len(got) != 2 || got[0] != "value" || got[1] != "error" {
		t.Fatalf("ResponseSubtypes() = %#v", got)
	}
	if err := transport.ValidateFlow(runtime.Flow{Entrypoint: runtime.Entrypoint{Config: map[string]any{"input": map[string]any{}}}}); err != nil {
		t.Fatalf("ValidateFlow() unexpected error: %v", err)
	}
	if err := transport.ValidateFlow(runtime.Flow{Entrypoint: runtime.Entrypoint{Config: map[string]any{"method": "POST"}}}); err == nil {
		t.Fatal("ValidateFlow() expected unsupported key error")
	}
}

func TestStartReturnsAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New().Start(ctx, runtime.TransportRuntime{})
	}()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
