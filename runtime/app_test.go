package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type fakeTransport struct {
	transportType string
	subtypes      []string
	start         func(context.Context, TransportRuntime) error
	shutdown      func(context.Context) error
}

func (t fakeTransport) Type() string { return t.transportType }
func (t fakeTransport) ResponseSubtypes() []string {
	return t.subtypes
}
func (t fakeTransport) ValidateFlow(flow Flow) error { return nil }
func (t fakeTransport) Start(ctx context.Context, rt TransportRuntime) error {
	if t.start != nil {
		return t.start(ctx, rt)
	}
	return nil
}
func (t fakeTransport) Shutdown(ctx context.Context) error {
	if t.shutdown != nil {
		return t.shutdown(ctx)
	}
	return nil
}

type fakeLoader struct{}

func (fakeLoader) Extensions() []string { return []string{"*.flow"} }
func (fakeLoader) Load(filePath string) (Flow, error) {
	transportType := fileBaseWithoutExt(filePath)
	return Flow{
		ID:         transportType,
		Entrypoint: Entrypoint{Type: transportType},
		Steps:      []Step{{ID: "respond", Body: `response.json({status: 200})`}},
	}, nil
}

type fakeEvaluator struct{}

func (fakeEvaluator) Eval(*Execution, string) (any, error) { return nil, nil }
func (fakeEvaluator) EvalWithEnv(*Execution, string, map[string]any) (any, error) {
	return nil, nil
}

type fakeStepExecutor struct{}

func (fakeStepExecutor) ExecuteStep(context.Context, *Execution, Step) (string, error) {
	return "", nil
}

type fakeStepRunner struct{}

func (fakeStepRunner) RunStep(context.Context, *Execution, StepInput) (StepOutput, error) {
	return StepOutput{}, nil
}

type shutdownOnlyPlugin struct {
	called *bool
}

func (p shutdownOnlyPlugin) Shutdown(Logger) error {
	*p.called = true
	return nil
}

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

func TestAppStart_ShutsDownOnContextCancellation(t *testing.T) {
	flowsDir := writeFlowFiles(t, "http")
	started := make(chan struct{})
	shutdownCalled := make(chan struct{})
	app := newLifecycleTestApp()
	if err := app.RegisterTransport(fakeTransport{
		transportType: "http",
		start: func(ctx context.Context, _ TransportRuntime) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
		shutdown: func(context.Context) error {
			close(shutdownCalled)
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Start(ctx, flowsDir) }()

	<-started
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	<-shutdownCalled
}

func TestAppStart_PropagatesTransportError(t *testing.T) {
	flowsDir := writeFlowFiles(t, "http")
	expected := errors.New("listen failed")
	app := newLifecycleTestApp()
	if err := app.RegisterTransport(fakeTransport{
		transportType: "http",
		start: func(context.Context, TransportRuntime) error {
			return expected
		},
	}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	err := app.Start(context.Background(), flowsDir)
	if !errors.Is(err, expected) {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestAppStart_RunsBlockingTransportsConcurrentlyAndShutsDownInReverseOrder(t *testing.T) {
	flowsDir := writeFlowFiles(t, "http", "cron")
	app := newLifecycleTestApp()
	started := make(chan string, 2)
	var shutdownOrder []string

	for _, transportType := range []string{"http", "cron"} {
		transportType := transportType
		if err := app.RegisterTransport(fakeTransport{
			transportType: transportType,
			start: func(ctx context.Context, _ TransportRuntime) error {
				started <- transportType
				<-ctx.Done()
				return ctx.Err()
			},
			shutdown: func(context.Context) error {
				shutdownOrder = append(shutdownOrder, transportType)
				return nil
			},
		}); err != nil {
			t.Fatalf("RegisterTransport failed: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Start(ctx, flowsDir) }()

	gotStarted := []string{<-started, <-started}
	slices.Sort(gotStarted)
	if !slices.Equal(gotStarted, []string{"cron", "http"}) {
		t.Fatalf("expected both transports to start, got %v", gotStarted)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	expectedShutdownOrder := []string{"cron", "http"}
	if !slices.Equal(shutdownOrder, expectedShutdownOrder) {
		t.Fatalf("shutdown order = %v, want %v", shutdownOrder, expectedShutdownOrder)
	}
}

func TestAppStart_ShutsDownPluginsOnStartupFailure(t *testing.T) {
	app := newLifecycleTestApp()
	shutdownCalled := false
	if err := app.Container.RegisterPlugin("cleanup", shutdownOnlyPlugin{called: &shutdownCalled}); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	err := app.Start(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !shutdownCalled {
		t.Fatal("expected plugin shutdown after startup failure")
	}
}

func newLifecycleTestApp() *App {
	return NewApp(
		NewContainer(NewLogger(nil)),
		fakeLoader{},
		fakeEvaluator{},
		fakeStepExecutor{},
		fakeStepRunner{},
		nil,
		func() ValueStore { return NewValueStore() },
	)
}

func writeFlowFiles(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name+".flow"), []byte("unused"), 0644); err != nil {
			t.Fatalf("write flow file: %v", err)
		}
	}
	return dir
}

func fileBaseWithoutExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}
