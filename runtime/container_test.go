package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingInitializerPlugin struct{}

func (p *failingInitializerPlugin) Initialize(_ Logger) error {
	return errors.New("boom")
}

func TestContainerInitialize_ReturnsNamedPluginError(t *testing.T) {
	container := NewContainer(NewLogger(nil))
	if err := container.RegisterPlugin("payments", &failingInitializerPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	err := container.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize to return an error")
	}
	if !strings.Contains(err.Error(), `plugin "payments" initialization failed`) {
		t.Fatalf("expected named plugin error, got %v", err)
	}
}
