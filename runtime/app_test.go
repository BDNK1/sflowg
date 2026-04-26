package runtime

import (
	"context"
	"testing"
)

type fakeTransport struct {
	transportType string
	subtypes      []string
}

func (t fakeTransport) Type() string { return t.transportType }
func (t fakeTransport) ResponseSubtypes() []string {
	return t.subtypes
}
func (t fakeTransport) ValidateFlow(flow Flow) error { return nil }
func (t fakeTransport) Start(ctx context.Context, rt TransportRuntime) error {
	return nil
}
func (t fakeTransport) Shutdown(ctx context.Context) error { return nil }

func TestGroupFlowsByTransport_SetsResponseSubtypes(t *testing.T) {
	app := &App{
		Container:  NewContainer(NewLogger(nil)),
		Flows:      map[string]Flow{"payments": {ID: "payments", Entrypoint: Entrypoint{Type: "http"}, Steps: []Step{{ID: "respond", Body: `response.json({status: 200})`}}}},
		transports: NewTransportRegistry(),
	}
	if err := app.RegisterTransport(fakeTransport{transportType: "http", subtypes: []string{"json", "text", "redirect"}}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	_, _, err := app.groupFlowsByTransport()
	if err != nil {
		t.Fatalf("groupFlowsByTransport failed: %v", err)
	}
	if got := app.Flows["payments"].ResponseSubtypes; len(got) != 3 || got[0] != "json" {
		t.Fatalf("response subtypes = %#v", got)
	}
}
