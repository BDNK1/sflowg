package flowtransport

import (
	"context"
	"fmt"

	"github.com/BDNK1/sflowg/core"
)

type Transport struct{}

func New() *Transport { return &Transport{} }

func (t *Transport) Type() string { return "flow" }

func (t *Transport) ResponseSubtypes() []string { return []string{"value", "error"} }

func (t *Transport) ValidateFlow(flow runtime.Flow) error {
	for key := range flow.Entrypoint.Config {
		if key != "input" {
			return fmt.Errorf("unsupported flow entrypoint key %q", key)
		}
	}
	return nil
}

func (t *Transport) Start(ctx context.Context, _ runtime.TransportRuntime) error {
	<-ctx.Done()
	return nil
}

func (t *Transport) Shutdown(context.Context) error { return nil }
