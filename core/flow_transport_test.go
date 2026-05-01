package runtime

import (
	"context"
	"testing"
)

func TestFlowTransportBasics(t *testing.T) {
	transport := NewFlowTransport()
	if transport.Type() != "flow" {
		t.Fatalf("Type() = %q, want flow", transport.Type())
	}
	if got := transport.ResponseContract().Subtypes(); len(got) != 2 || got[0] != "value" || got[1] != "error" {
		t.Fatalf("ResponseContract().Subtypes() = %#v", got)
	}
	if err := transport.ValidateFlow(Flow{Entrypoint: Entrypoint{Config: map[string]any{"input": map[string]any{}}}}); err != nil {
		t.Fatalf("ValidateFlow() unexpected error: %v", err)
	}
	if err := transport.ValidateFlow(Flow{Entrypoint: Entrypoint{Config: map[string]any{"method": "POST"}}}); err == nil {
		t.Fatal("ValidateFlow() expected unsupported key error")
	}
}

func TestFlowTransportStartReturnsAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewFlowTransport().Start(ctx, TransportRuntime{})
	}()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
