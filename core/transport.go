package core

import (
	"context"
	"fmt"
)

// Transport owns protocol-specific serving and response dispatch.
type Transport interface {
	Type() string
	ResponseSubtypes() []string
	ValidateFlow(flow Flow) error
	Start(ctx context.Context, runtime TransportRuntime) error
	Shutdown(ctx context.Context) error
}

// TransportRuntime contains the shared runtime services a transport needs to
// execute its assigned flows.
type TransportRuntime struct {
	Container        *Container
	Executor         *Executor
	Flows            []Flow
	GlobalProperties map[string]any
	NewValueStore    func() ValueStore
}

// TransportRegistry stores transports by entrypoint type while preserving
// registration order for lifecycle operations.
type TransportRegistry struct {
	byType map[string]Transport
	order  []Transport
}

func NewTransportRegistry() *TransportRegistry {
	return &TransportRegistry{
		byType: make(map[string]Transport),
	}
}

func (r *TransportRegistry) Register(transport Transport) error {
	if transport == nil {
		return fmt.Errorf("transport is nil")
	}
	transportType := transport.Type()
	if transportType == "" {
		return fmt.Errorf("transport type is empty")
	}
	if _, exists := r.byType[transportType]; exists {
		return fmt.Errorf("transport %q is already registered", transportType)
	}
	r.byType[transportType] = transport
	r.order = append(r.order, transport)
	return nil
}

func (r *TransportRegistry) Get(transportType string) (Transport, bool) {
	transport, ok := r.byType[transportType]
	return transport, ok
}
