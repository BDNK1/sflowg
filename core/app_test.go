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
func (t fakeTransport) ResponseContract() ResponseContract {
	contracts := make([]ResponseSubtypeContract, 0, len(t.subtypes))
	for _, subtype := range t.subtypes {
		contracts = append(contracts, ResponseSubtypeContract{Name: subtype, Args: ResponseArgsMap})
	}
	return NewResponseContract(t.transportType, contracts...)
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

type onErrorLoader struct{}

func (onErrorLoader) Extensions() []string { return []string{"*.flow"} }
func (onErrorLoader) Load(filePath string) (Flow, error) {
	transportType := fileBaseWithoutExt(filePath)
	return Flow{
		ID:          transportType,
		Entrypoint:  Entrypoint{Type: transportType},
		Steps:       []Step{{ID: "respond", Body: `response.json({status: 200})`}},
		OnErrorBody: `response.json({status: 500})`,
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

type fakeCompiler struct{}

func (fakeCompiler) CompileFlow(_ context.Context, flow *Flow, _ *Container) error {
	if flow.ResponseContract.IsZero() {
		return errors.New("compiler received flow without response contract")
	}
	flow.OnErrorCompiled = "compiled on_error"
	for i := range flow.Steps {
		flow.Steps[i].Compiled = "compiled step"
		flow.Steps[i].StoreKeys = []string{}
	}
	return nil
}

type deadlineCapturingStepRunner struct {
	deadlineSet *bool
}

func (r deadlineCapturingStepRunner) RunStep(ctx context.Context, _ *Execution, _ StepInput) (StepOutput, error) {
	_, ok := ctx.Deadline()
	*r.deadlineSet = ok
	return StepOutput{Response: &ResponseDescriptor{Subtype: "value", Args: map[string]any{"ok": true}}}, nil
}

type shutdownOnlyPlugin struct {
	called *bool
}

func (p shutdownOnlyPlugin) Shutdown(Logger) error {
	*p.called = true
	return nil
}

func TestValidateAndAnnotateFlowsByTransport_SetsResponseSubtypes(t *testing.T) {
	app := &App{
		Container:  NewContainer(NewLogger(nil)),
		Flows:      map[string]Flow{"payments": {ID: "payments", Entrypoint: Entrypoint{Type: "http"}, Steps: []Step{{ID: "respond", Body: `response.json({status: 200})`}}}},
		transports: NewTransportRegistry(),
	}
	if err := app.RegisterTransport(fakeTransport{transportType: "http", subtypes: []string{"json", "text", "redirect"}}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	annotated, active, err := app.annotateFlowsByTransport(app.Flows)
	if err != nil {
		t.Fatalf("annotateFlowsByTransport failed: %v", err)
	}
	if len(active) != 1 || active[0].Type() != "http" {
		t.Fatalf("active transports = %#v, want http", active)
	}
	if got := annotated["payments"].ResponseSubtypes; len(got) != 3 || got[0] != "json" {
		t.Fatalf("response subtypes = %#v", got)
	}
	if got := app.Flows["payments"].ResponseSubtypes; len(got) != 0 {
		t.Fatalf("annotateFlowsByTransport should not mutate input app flows, got %#v", got)
	}
}

func TestPrepareFlows_DoesNotPublishIntermediateFlows(t *testing.T) {
	flowsDir := writeFlowFiles(t, "http")
	app := NewApp(
		NewContainer(NewLogger(nil)),
		onErrorLoader{},
		fakeEvaluator{},
		fakeStepExecutor{},
		fakeStepRunner{},
		fakeCompiler{},
		func() ValueStore { return NewValueStore() },
	)
	if err := app.RegisterTransport(fakeTransport{transportType: "http", subtypes: []string{"json"}}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	prepared, err := app.prepareFlows(context.Background(), flowsDir)
	if err != nil {
		t.Fatalf("prepareFlows failed: %v", err)
	}
	if len(app.Flows) != 0 {
		t.Fatalf("prepareFlows should not publish intermediate flows, got %#v", app.Flows)
	}
	flow := prepared.flows["http"]
	if flow.OnErrorCompiled == nil {
		t.Fatal("prepared flow is missing compiled on_error")
	}
	if flow.ResponseContract.IsZero() {
		t.Fatal("prepared flow is missing response contract")
	}
}

func TestGroupFlowsByTransport_UsesProvidedFinalFlows(t *testing.T) {
	flows := map[string]Flow{
		"http": {
			ID:              "http",
			Entrypoint:      Entrypoint{Type: "http"},
			OnErrorCompiled: "compiled on_error",
			Steps:           []Step{{ID: "respond", Compiled: "compiled step"}},
		},
	}
	grouped := groupFlowsByTransport(flows, []Transport{fakeTransport{transportType: "http"}})
	if got := grouped["http"]; len(got) != 1 {
		t.Fatalf("grouped http flows = %#v", got)
	}
	if grouped["http"][0].OnErrorCompiled == nil {
		t.Fatal("grouped flow lost compiled on_error")
	}
	if flows["http"].OnErrorCompiled == nil {
		t.Fatal("groupFlowsByTransport should not mutate input flows")
	}
}

func TestAppStart_TransportReceivesCompiledFlowFields(t *testing.T) {
	flowsDir := writeFlowFiles(t, "http")
	received := make(chan Flow, 1)
	app := NewApp(
		NewContainer(NewLogger(nil)),
		onErrorLoader{},
		fakeEvaluator{},
		fakeStepExecutor{},
		fakeStepRunner{},
		fakeCompiler{},
		func() ValueStore { return NewValueStore() },
	)
	if err := app.RegisterTransport(fakeTransport{
		transportType: "http",
		subtypes:      []string{"json"},
		start: func(ctx context.Context, rt TransportRuntime) error {
			received <- rt.Flows[0]
			<-ctx.Done()
			return ctx.Err()
		},
	}); err != nil {
		t.Fatalf("RegisterTransport failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Start(ctx, flowsDir) }()

	flow := <-received
	cancel()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatalf("expected context canceled")
	}
	if flow.OnErrorCompiled == nil {
		t.Fatal("transport received flow without compiled on_error")
	}
	if flow.Steps[0].Compiled == nil {
		t.Fatal("transport received flow without compiled step")
	}
}

func TestInvokeSubflow_HonorsTargetTimeout(t *testing.T) {
	deadlineSet := false
	app := NewApp(
		NewContainer(NewLogger(nil)),
		fakeLoader{},
		fakeEvaluator{},
		fakeStepExecutor{},
		deadlineCapturingStepRunner{deadlineSet: &deadlineSet},
		nil,
		func() ValueStore { return NewValueStore() },
	)
	parentFlow := &Flow{ID: "caller", Entrypoint: Entrypoint{Type: "http"}}
	parent := NewExecution(parentFlow, app.Container, nil, NewValueStore())
	target := &Flow{
		ID:               "sub",
		Entrypoint:       Entrypoint{Type: "flow"},
		Timeout:          25,
		ResponseSubtypes: []string{"value", "error"},
		Steps:            []Step{{ID: "__return", Body: `response.value({ok: true})`}},
	}

	if _, err := app.InvokeSubflow(parent, target, nil); err != nil {
		t.Fatalf("InvokeSubflow() error = %v", err)
	}
	if !deadlineSet {
		t.Fatal("expected subflow step context to have deadline")
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
