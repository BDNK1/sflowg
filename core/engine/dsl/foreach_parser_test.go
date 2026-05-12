package dsl

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/core"
)

func TestParseForeachSequential(t *testing.T) {
	flow, err := Parse(`
foreach request.body.items as item {
  step normalize { {sku: item.sku} }
  collect normalize.sku as skus
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(flow.Nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(flow.Nodes))
	}
	node := flow.Nodes[0]
	if node.Kind != runtime.FlowNodeForeach || node.ID != "__foreach_1" {
		t.Fatalf("unexpected node: %#v", node)
	}
	block := node.Foreach
	if block == nil {
		t.Fatal("foreach block is nil")
	}
	if block.Expr != "request.body.items" || block.ItemVar != "item" || block.Parallel {
		t.Fatalf("unexpected foreach block: %#v", block)
	}
	if len(block.Steps) != 1 || block.Steps[0].ID != "normalize" {
		t.Fatalf("unexpected foreach steps: %#v", block.Steps)
	}
	if len(block.Collects) != 1 || block.Collects[0].Expr != "normalize.sku" || block.Collects[0].Alias != "skus" {
		t.Fatalf("unexpected foreach collects: %#v", block.Collects)
	}
}

func TestParseForeachBatch(t *testing.T) {
	flow, err := Parse(`
foreach request.body.items batch 25 as items {
  step bulk_insert { {count: len(items)} }
  collect bulk_insert.count as batch_counts
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	block := flow.Nodes[0].Foreach
	if block.BatchSize != 25 || block.ItemVar != "items" {
		t.Fatalf("unexpected batch foreach: %#v", block)
	}
}

func TestParseParallelForeach(t *testing.T) {
	flow, err := Parse(`
parallel(max_in_flight: 4, on_failure: "fail_fast") foreach request.body.items as item {
  async step enrich { {body: item.id} }
  collect enrich.body as enriched
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	node := flow.Nodes[0]
	if node.Kind != runtime.FlowNodeForeach {
		t.Fatalf("node kind = %q, want foreach", node.Kind)
	}
	block := node.Foreach
	if !block.Parallel {
		t.Fatal("expected parallel foreach")
	}
	if block.Options.MaxInFlight != 4 || block.Options.OnFailure != runtime.OnFailureFailFast {
		t.Fatalf("unexpected options: %#v", block.Options)
	}
	if len(block.Steps) != 1 || !block.Steps[0].Async {
		t.Fatalf("expected async body step: %#v", block.Steps)
	}
}

func TestParseParallelDefaultForeach(t *testing.T) {
	flow, err := Parse(`
parallel() foreach request.body.items as item {
  collect item.id as ids
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	block := flow.Nodes[0].Foreach
	if !block.Parallel {
		t.Fatal("expected parallel foreach")
	}
	if block.Options.MaxInFlight != 0 || block.Options.OnFailure != "" {
		t.Fatalf("default options should remain unset for runtime defaults: %#v", block.Options)
	}
}

func TestParseForeachMultipleCollects(t *testing.T) {
	flow, err := Parse(`
foreach rows as row {
  step normalize { {id: row.id, amount: row.amount} }
  collect normalize.id as ids
  collect normalize.amount as amounts
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	collects := flow.Nodes[0].Foreach.Collects
	if len(collects) != 2 || collects[0].Alias != "ids" || collects[1].Alias != "amounts" {
		t.Fatalf("unexpected collects: %#v", collects)
	}
}

func TestParseForeachRejectsInvalidConstructs(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name:    "invalid batch size",
			source:  `foreach rows batch 0 as row { collect row as rows }`,
			wantErr: "batch size",
		},
		{
			name:    "nested foreach",
			source:  `foreach rows as row { foreach row.items as item { collect item as items } }`,
			wantErr: "nested foreach",
		},
		{
			name:    "nested parallel",
			source:  `foreach rows as row { parallel { step a { nil } } }`,
			wantErr: "nested parallel",
		},
		{
			name:    "return",
			source:  `foreach rows as row { return response.json({body: row}) }`,
			wantErr: "return is not allowed",
		},
		{
			name:    "compensate",
			source:  `foreach rows as row { step a { nil } compensate { nil } }`,
			wantErr: "cannot have compensate",
		},
		{
			name:    "duplicate step",
			source:  `foreach rows as row { step a { nil } step a { nil } }`,
			wantErr: "duplicate foreach body step",
		},
		{
			name:    "duplicate collect",
			source:  `foreach rows as row { collect row.id as ids collect row.sku as ids }`,
			wantErr: "duplicate foreach collect alias",
		},
		{
			name:    "dotted alias",
			source:  `foreach rows as row { collect row.id as result.ids }`,
			wantErr: "simple identifier",
		},
		{
			name:    "parallel foreach without parentheses",
			source:  `parallel foreach rows as row { collect row as rows }`,
			wantErr: "requires parentheses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
