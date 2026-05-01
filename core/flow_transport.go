package runtime

import (
	"context"
	"fmt"
)

type FlowTransport struct{}

func NewFlowTransport() *FlowTransport { return &FlowTransport{} }

func (t *FlowTransport) Type() string { return "flow" }

func (t *FlowTransport) ResponseContract() ResponseContract {
	return FlowResponseContract()
}

func (t *FlowTransport) ValidateFlow(flow Flow) error {
	for key := range flow.Entrypoint.Config {
		if key != "input" {
			return fmt.Errorf("unsupported flow entrypoint key %q", key)
		}
	}
	return nil
}

func (t *FlowTransport) Start(ctx context.Context, _ TransportRuntime) error {
	<-ctx.Done()
	return nil
}

func (t *FlowTransport) Shutdown(context.Context) error { return nil }
