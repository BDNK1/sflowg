package dsl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/core"
	"github.com/deepnoodle-ai/risor/v2/pkg/bytecode"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type compileTestPlugin struct{}

func (p *compileTestPlugin) Get(_ *core.Execution, args map[string]any) (map[string]any, error) {
	return args, nil
}

func newCompileTestContainer(t *testing.T) *core.Container {
	t.Helper()

	container := core.NewContainer(core.NewLogger(nil))
	if err := container.RegisterPlugin("postgres", &compileTestPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin() error = %v", err)
	}

	metrics, err := core.NewTestMetricsWithReader(sdkmetric.NewManualReader(), map[string]core.UserMetricDecl{
		"payment_attempts": {Type: "counter"},
	})
	if err != nil {
		t.Fatalf("NewTestMetricsWithReader() error = %v", err)
	}
	container.SetMetrics(metrics)

	return container
}

func TestCompileFlow_CompilesAllBodies(t *testing.T) {
	flow := &core.Flow{
		ID: "payments",
		Steps: []core.Step{
			{
				ID:             "create_order",
				Body:           `postgres.get({query: "select 1"})`,
				FallbackBody:   `response.json({status: 202})`,
				CompensateBody: `log.info("undo", compensation.step)`,
			},
			{
				ID:   "__return",
				Body: `response.json({status: 200, body: {ok: create_order != nil}})`,
			},
		},
		OnErrorBody: `response.json({status: 500, body: {error: error.code}})`,
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}

	if _, ok := flow.Steps[0].Compiled.(*bytecode.Code); !ok {
		t.Fatalf("primary body was not compiled: %#v", flow.Steps[0].Compiled)
	}
	if flow.Steps[0].StoreKeys == nil {
		t.Fatalf("primary body should record zero store keys explicitly, got nil")
	}
	if _, ok := flow.Steps[0].FallbackCompiled.(*bytecode.Code); !ok {
		t.Fatalf("fallback body was not compiled: %#v", flow.Steps[0].FallbackCompiled)
	}
	if _, ok := flow.Steps[0].CompensateCompiled.(*bytecode.Code); !ok {
		t.Fatalf("compensation body was not compiled: %#v", flow.Steps[0].CompensateCompiled)
	}
	if _, ok := flow.Steps[1].Compiled.(*bytecode.Code); !ok {
		t.Fatalf("__return body was not compiled: %#v", flow.Steps[1].Compiled)
	}
	if _, ok := flow.OnErrorCompiled.(*bytecode.Code); !ok {
		t.Fatalf("on_error body was not compiled: %#v", flow.OnErrorCompiled)
	}
}

func TestCompileFlow_BodiesWithoutStoreReadsUseEmptyStoreKeysSlice(t *testing.T) {
	flow := &core.Flow{
		ID: "payments",
		Steps: []core.Step{
			{ID: "constant", Body: `42`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}

	if flow.Steps[0].StoreKeys == nil {
		t.Fatal("expected empty store key slice, got nil")
	}
	if len(flow.Steps[0].StoreKeys) != 0 {
		t.Fatalf("expected zero store keys, got %#v", flow.Steps[0].StoreKeys)
	}
}

func TestCompileFlow_UsesFlowResponseSubtypes(t *testing.T) {
	flow := &core.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []core.Step{
			{ID: "respond", Body: `response.text({status: 200, body: "ok"})`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	if _, ok := flow.Steps[0].Compiled.(*bytecode.Code); !ok {
		t.Fatalf("body was not compiled: %#v", flow.Steps[0].Compiled)
	}
}

func TestCompileFlow_RejectsInvalidResponseSubtype(t *testing.T) {
	flow := &core.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []core.Step{
			{ID: "respond", Body: `response.ack()`},
		},
	}

	compiler := NewCompiler()
	err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t))
	if err == nil {
		t.Fatal("expected CompileFlow() to reject response.ack")
	}
	if !strings.Contains(err.Error(), "ack") {
		t.Fatalf("expected error to mention ack, got %v", err)
	}
}

func TestCompileFlow_DoesNotRejectResponseTextInStringLiteral(t *testing.T) {
	flow := &core.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []core.Step{
			{ID: "literal", Body: `"response.ack({status: 200})"`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
}

func TestCompileFlow_StoresOnlyReferencedKeys(t *testing.T) {
	flow := &core.Flow{
		ID: "payments",
		Steps: []core.Step{
			{
				ID:   "create_order",
				Body: `{email: request.body.email, api: properties.api_key}`,
			},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}

	want := []string{"properties", "request"}
	got := flow.Steps[0].StoreKeys
	if len(got) != len(want) {
		t.Fatalf("StoreKeys len = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("StoreKeys = %#v, want %#v", got, want)
		}
	}
}

func TestCompileFlow_InvalidSyntaxFails(t *testing.T) {
	flow := &core.Flow{
		ID: "payments",
		Steps: []core.Step{
			{ID: "broken", Body: `let x =`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err == nil {
		t.Fatal("expected compile error, got nil")
	}
}

func TestFlowLoaderAndCompiler_CompilesSynthesizedReturnStep(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "payments.flow")
	source := `entrypoint.http { method: GET path: /payments }

step load {
    {ok: true}
}

return response.json({status: 200, body: {ok: load.ok}})
`
	if err := os.WriteFile(flowPath, []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loader := NewFlowLoader()
	flow, err := loader.Load(flowPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}

	last := flow.Steps[len(flow.Steps)-1]
	if last.ID != "__return" {
		t.Fatalf("last step ID = %q, want __return", last.ID)
	}
	if _, ok := last.Compiled.(*bytecode.Code); !ok {
		t.Fatalf("__return body was not compiled: %#v", last.Compiled)
	}
}
