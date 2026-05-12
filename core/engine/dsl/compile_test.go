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

func (p *compileTestPlugin) Get(_ *runtime.Execution, args map[string]any) (map[string]any, error) {
	return args, nil
}

func newCompileTestContainer(t *testing.T) *runtime.Container {
	t.Helper()

	container := runtime.NewContainer(runtime.NewLogger(nil))
	if err := container.RegisterPlugin("postgres", &compileTestPlugin{}); err != nil {
		t.Fatalf("RegisterPlugin() error = %v", err)
	}

	metrics, err := runtime.NewTestMetricsWithReader(sdkmetric.NewManualReader(), map[string]runtime.UserMetricDecl{
		"payment_attempts": {Type: "counter"},
	})
	if err != nil {
		t.Fatalf("NewTestMetricsWithReader() error = %v", err)
	}
	container.SetMetrics(metrics)

	return container
}

func TestCompileFlow_CompilesAllBodies(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
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

func TestCompileFlow_CompensateCanReadCompensatedStepResult(t *testing.T) {
	flow, err := Parse(`
step insert_payment {
	{row: {id: 123}}
} compensate {
	postgres.get({
		query: "delete from payments where id = $1",
		params: [insert_payment.row.id]
	})
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	flow.ID = "payments"

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	if _, ok := flow.Steps[0].CompensateCompiled.(*bytecode.Code); !ok {
		t.Fatalf("compensation body was not compiled: %#v", flow.Steps[0].CompensateCompiled)
	}
}

func TestCompileFlow_AcceptsHeaderCallPluginSugar(t *testing.T) {
	flow, err := Parse(`step fetch_order as postgres.get {
	query: "select 1"
	params: []
}`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	flow.ID = "payments"

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	if _, ok := flow.Steps[0].Compiled.(*bytecode.Code); !ok {
		t.Fatalf("body was not compiled: %#v", flow.Steps[0].Compiled)
	}
}

func TestCompileFlow_BodiesWithoutStoreReadsUseEmptyStoreKeysSlice(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
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
	flow := &runtime.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []runtime.Step{
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
	flow := &runtime.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []runtime.Step{
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
	flow := &runtime.Flow{
		ID:               "payments",
		ResponseSubtypes: []string{"json", "text", "redirect"},
		Steps: []runtime.Step{
			{ID: "literal", Body: `"response.ack({status: 200})"`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
}

func TestCompileFlow_StoresOnlyReferencedKeys(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
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
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "broken", Body: `let x =`},
		},
	}

	compiler := NewCompiler()
	if err := compiler.CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err == nil {
		t.Fatal("expected compile error, got nil")
	}
}

func TestCompileFlow_PopulatesAsyncDepsAcrossSurfaces(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "prefetch", Async: true, Body: `{ok: true}`},
			{
				ID:           "consume",
				Condition:    `prefetch.ok`,
				Body:         `{body: prefetch.body}`,
				Retry:        &runtime.RetryConfig{MaxAttempts: 2, When: `prefetch.retry`},
				FallbackBody: `{fallback: prefetch.fallback}`,
			},
		},
	}

	if err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}

	got := flow.Steps[1].AsyncDeps
	if len(got) != 1 || got[0] != "prefetch" {
		t.Fatalf("AsyncDeps = %#v, want [prefetch]", got)
	}
}

func TestCompileFlow_RejectsForwardAsyncReference(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "consume", Body: `{body: prefetch.body}`},
			{ID: "prefetch", Async: true, Body: `{ok: true}`},
		},
	}

	err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t))
	if err == nil {
		t.Fatal("expected forward async reference error")
	}
	if !strings.Contains(err.Error(), "before it is spawned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileFlow_RejectsForeachForwardParentAsyncReference(t *testing.T) {
	tests := []struct {
		name    string
		foreach runtime.ForeachBlock
	}{
		{
			name: "source",
			foreach: runtime.ForeachBlock{
				Expr:    `prefetch.body`,
				ItemVar: "row",
			},
		},
		{
			name: "body",
			foreach: runtime.ForeachBlock{
				Expr:    `[]`,
				ItemVar: "row",
				Steps: []runtime.Step{{
					ID:   "use_prefetch",
					Body: `{body: prefetch.body}`,
				}},
			},
		},
		{
			name: "condition",
			foreach: runtime.ForeachBlock{
				Expr:    `[]`,
				ItemVar: "row",
				Steps: []runtime.Step{{
					ID:        "use_prefetch",
					Condition: `prefetch.body != nil`,
					Body:      `{ok: true}`,
				}},
			},
		},
		{
			name: "retry",
			foreach: runtime.ForeachBlock{
				Expr:    `[]`,
				ItemVar: "row",
				Steps: []runtime.Step{{
					ID:    "use_prefetch",
					Body:  `{ok: true}`,
					Retry: &runtime.RetryConfig{MaxAttempts: 2, When: `prefetch.body != nil`},
				}},
			},
		},
		{
			name: "collect",
			foreach: runtime.ForeachBlock{
				Expr:    `[]`,
				ItemVar: "row",
				Collects: []runtime.ForeachCollect{{
					Expr:  `prefetch.body`,
					Alias: "collected",
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := &runtime.Flow{
				ID: "payments",
				Nodes: []runtime.FlowNode{
					{ID: "__foreach_1", Kind: runtime.FlowNodeForeach, Foreach: &tt.foreach},
					{ID: "prefetch", Kind: runtime.FlowNodeStep, Step: &runtime.Step{ID: "prefetch", Async: true, Body: `{ok: true}`}},
				},
			}

			err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t))
			if err == nil {
				t.Fatal("expected forward async reference error")
			}
			if !strings.Contains(err.Error(), "before it is spawned") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompileFlow_AllowsForeachLocalAsyncReferenceWithSameFutureParentName(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Nodes: []runtime.FlowNode{
			{
				ID:   "__foreach_1",
				Kind: runtime.FlowNodeForeach,
				Foreach: &runtime.ForeachBlock{
					Expr:    `[]`,
					ItemVar: "row",
					Steps: []runtime.Step{
						{ID: "prefetch", Async: true, Body: `{ok: true}`},
						{ID: "use_prefetch", Body: `{body: prefetch.body}`},
					},
				},
			},
			{ID: "prefetch", Kind: runtime.FlowNodeStep, Step: &runtime.Step{ID: "prefetch", Async: true, Body: `{ok: true}`}},
		},
	}

	if err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
}

func TestCompileFlow_RejectsForeachCompensation(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Nodes: []runtime.FlowNode{{
			ID:   "__foreach_1",
			Kind: runtime.FlowNodeForeach,
			Foreach: &runtime.ForeachBlock{
				Expr:    `[]`,
				ItemVar: "row",
				Steps: []runtime.Step{{
					ID:             "charge",
					Body:           `{ok: true}`,
					CompensateBody: `{ok: true}`,
				}},
			},
		}},
	}

	err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t))
	if err == nil {
		t.Fatal("expected foreach compensation rejection")
	}
	if !strings.Contains(err.Error(), "cannot have compensate block") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileFlow_ForeachCollectAliasVisibility(t *testing.T) {
	flow, err := Parse(`
foreach request.body.items as item {
  collect item.id as ids
}
step after { {ids: ids} }
on_error { response.json({status: 500, body: {ids: ids}}) }
return response.json({status: 200, body: {ids: ids}})
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	after := flow.Nodes[1].Step
	if len(after.StoreKeys) != 1 || after.StoreKeys[0] != "ids" {
		t.Fatalf("after StoreKeys = %#v, want [ids]", after.StoreKeys)
	}
}

func TestCompileFlow_ForeachRejectsInvalidVisibility(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "upstream step reads later collect alias",
			source: `
step before { {ids: ids} }
foreach request.body.items as item {
  collect item.id as ids
}
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "downstream step reads body step",
			source: `
foreach request.body.items as item {
  step normalize { {id: item.id} }
  collect normalize.id as ids
}
step after { {id: normalize.id} }
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "return reads body step",
			source: `
foreach request.body.items as item {
  step normalize { {id: item.id} }
}
return response.json({status: 200, body: {id: normalize.id}})
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "on_error reads body step",
			source: `
foreach request.body.items as item {
  step normalize { {id: item.id} }
}
on_error { response.json({status: 500, body: {id: normalize.id}}) }
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "collect reads same foreach collect alias",
			source: `
foreach request.body.items as item {
  collect item.id as ids
  collect ids as duplicate
}
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "source reads downstream step",
			source: `
foreach after.items as item {
  collect item.id as ids
}
step after { {items: []} }
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "body reads downstream step",
			source: `
foreach request.body.items as item {
  step use_after { {value: after.value} }
}
step after { {value: true} }
`,
			wantErr: "outside lexical scope",
		},
		{
			name: "collect reads downstream step",
			source: `
foreach request.body.items as item {
  collect after.value as values
}
step after { {value: true} }
`,
			wantErr: "outside lexical scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			err = NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestCompileFlow_ForeachLocalAsyncDependencies(t *testing.T) {
	flow, err := Parse(`
async step parent_prefetch { {body: "parent"} }
foreach request.body.items as item {
  async step enrich { {body: item.id} }
  step use_local { {body: enrich.body, parent: parent_prefetch.body} }
  collect enrich.body as enriched
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	steps := flow.Nodes[1].Foreach.Steps
	if len(steps[1].AsyncDeps) != 2 || steps[1].AsyncDeps[0] != "enrich" || steps[1].AsyncDeps[1] != "parent_prefetch" {
		t.Fatalf("use_local AsyncDeps = %#v, want [enrich parent_prefetch]", steps[1].AsyncDeps)
	}
	collect := flow.Nodes[1].Foreach.Collects[0]
	if len(collect.StoreKeys) != 1 || collect.StoreKeys[0] != "enrich" {
		t.Fatalf("collect StoreKeys = %#v, want [enrich]", collect.StoreKeys)
	}
}

func TestCompileFlow_ForeachRejectsLocalForwardAsyncReference(t *testing.T) {
	flow, err := Parse(`
foreach request.body.items as item {
  step use_later { {body: enrich.body} }
  async step enrich { {body: item.id} }
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	err = NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t))
	if err == nil || !strings.Contains(err.Error(), "before it is spawned") {
		t.Fatalf("expected forward async error, got %v", err)
	}
}

func TestCompileFlow_ForeachRejectsShadowingAndReservedIDs(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "local async shadows prior parent async",
			source: `
async step prefetch { {ok: true} }
foreach request.body.items as item {
  async step prefetch { {ok: true} }
}
`,
		},
		{
			name: "body step shadows prior parent",
			source: `
step existing { {ok: true} }
foreach request.body.items as item {
  step existing { {ok: true} }
}
`,
		},
		{
			name: "loop variable shadows framework",
			source: `
foreach request.body.items as request {
  collect request.id as ids
}
`,
		},
		{
			name: "collect alias shadows framework",
			source: `
foreach request.body.items as item {
  collect item.id as request
}
`,
		},
		{
			name: "body step reserved prefix",
			source: `
foreach request.body.items as item {
  step __foreach_body { {ok: true} }
}
`,
		},
		{
			name: "loop variable reserved prefix",
			source: `
foreach request.body.items as __foreach_item {
  collect __foreach_item.id as ids
}
`,
		},
		{
			name: "collect alias reserved prefix",
			source: `
foreach request.body.items as item {
  collect item.id as __foreach_ids
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if err := NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err == nil {
				t.Fatal("expected CompileFlow() error")
			}
		})
	}
}

func TestCompileFlow_ForeachRejectsResponseAndUserNext(t *testing.T) {
	tests := []string{
		`foreach request.body.items as item { step respond { response.json({status: 200}) } }`,
		`foreach request.body.items as item { step route { {__next: "done"} } } step done { nil }`,
	}
	for _, source := range tests {
		flow, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", source, err)
		}
		err = NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t))
		if err == nil {
			t.Fatalf("CompileFlow(%q) succeeded, want error", source)
		}
		if !strings.Contains(err.Error(), "response") && !strings.Contains(err.Error(), "__next") {
			t.Fatalf("unexpected error for %q: %v", source, err)
		}
	}
}

func TestCompileFlow_ForeachStoresCompiledExpressions(t *testing.T) {
	flow, err := Parse(`
step before { {items: request.body.items} }
foreach before.items as item {
  step normalize { {id: item.id} }
  collect normalize.id as ids
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := NewCompiler().CompileFlow(context.Background(), &flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	block := flow.Nodes[1].Foreach
	if _, ok := block.ExprProgram.(*bytecode.Code); !ok {
		t.Fatalf("ExprProgram = %#v, want bytecode", block.ExprProgram)
	}
	if len(block.ExprStoreKeys) != 1 || block.ExprStoreKeys[0] != "before" {
		t.Fatalf("ExprStoreKeys = %#v, want [before]", block.ExprStoreKeys)
	}
	if _, ok := block.Collects[0].ExprProgram.(*bytecode.Code); !ok {
		t.Fatalf("Collect ExprProgram = %#v, want bytecode", block.Collects[0].ExprProgram)
	}
	if len(block.Collects[0].StoreKeys) != 1 || block.Collects[0].StoreKeys[0] != "normalize" {
		t.Fatalf("Collect StoreKeys = %#v, want [normalize]", block.Collects[0].StoreKeys)
	}
	if len(block.ParentStoreKeys) != 1 || block.ParentStoreKeys[0] != "before" {
		t.Fatalf("ParentStoreKeys = %#v, want [before]", block.ParentStoreKeys)
	}
}

func TestCompileFlow_AllowsReturnAndOnErrorAsyncReferences(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "consume", Body: `{ok: true}`},
			{ID: "prefetch", Async: true, Body: `{ok: true}`},
		},
		Return:      runtime.Return{Body: `response.json({status: 200, body: {ok: prefetch.ok}})`},
		OnErrorBody: `response.json({status: 500, body: {ok: prefetch.ok}})`,
	}

	if err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t)); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
}

func TestCompileFlow_RejectsReturnStepBeforeAsyncReference(t *testing.T) {
	flow := &runtime.Flow{
		ID: "payments",
		Steps: []runtime.Step{
			{ID: "__return", Body: `response.json({status: 200, body: {ok: prefetch.ok}})`},
			{ID: "prefetch", Async: true, Body: `{ok: true}`},
		},
	}

	err := NewCompiler().CompileFlow(context.Background(), flow, newCompileTestContainer(t))
	if err == nil {
		t.Fatal("expected forward async reference error")
	}
	if !strings.Contains(err.Error(), "before it is spawned") {
		t.Fatalf("unexpected error: %v", err)
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

func TestCompileFlow_CronTriggerKeysCompile(t *testing.T) {
	flow := &runtime.Flow{
		ID:               "reconcile",
		Entrypoint:       runtime.Entrypoint{Type: "cron"},
		ResponseContract: runtime.CronResponseContract(),
		Steps: []runtime.Step{
			{ID: "log_tick", Body: `log.info("tick", {scheduled_at: trigger.scheduled_at, fire_count: trigger.fire_count})`},
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
