package dsl

import (
	"context"
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/core"
)

func TestParseParallelBlockWithOptionsAndAsyncBranch(t *testing.T) {
	flow, err := Parse(`
parallel(max_in_flight: 3, on_failure: "fail_fast") {
  step enrich { {tier: "gold"} }
  async step prefetch { {body: "ok"} } fallback { {body: nil} }
}
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(flow.Nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(flow.Nodes))
	}
	node := flow.Nodes[0]
	if node.Kind != runtime.FlowNodeParallel || node.ID != "__parallel_1" {
		t.Fatalf("unexpected node: %#v", node)
	}
	if node.Parallel.Options.MaxInFlight != 3 || node.Parallel.Options.OnFailure != runtime.OnFailureFailFast {
		t.Fatalf("unexpected options: %#v", node.Parallel.Options)
	}
	if len(node.Parallel.Branches) != 2 || !node.Parallel.Branches[1].Async {
		t.Fatalf("unexpected branches: %#v", node.Parallel.Branches)
	}
	if node.Parallel.Branches[1].FallbackBody == "" {
		t.Fatal("expected fallback body")
	}
}

func TestParseParallelRejectsInvalidBranchConstructs(t *testing.T) {
	cases := []string{
		`parallel { step a { nil } compensate { nil } }`,
		`parallel(on_failure: "bad") {}`,
		`parallel { parallel { step a { nil } } }`,
	}
	for _, source := range cases {
		if _, err := Parse(source); err == nil {
			t.Fatalf("Parse(%q) succeeded, want error", source)
		}
	}
}

func TestCompileParallelBranchIDsAndPeerRefs(t *testing.T) {
	flow, err := Parse(`
step before { {ok: true} }
parallel {
  step left { {seen: before.ok} }
  async step right { {ok: true} }
}
step after { {value: right} }
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := NewCompiler().CompileFlow(context.Background(), &flow, runtime.NewContainer(runtime.NewLogger(nil))); err != nil {
		t.Fatalf("CompileFlow() error = %v", err)
	}
	branch := flow.Nodes[1].Parallel.Branches[0]
	if len(branch.StoreKeys) != 1 || branch.StoreKeys[0] != "before" {
		t.Fatalf("left StoreKeys = %#v, want before", branch.StoreKeys)
	}
	after := flow.Nodes[2].Step
	if len(after.AsyncDeps) != 1 || after.AsyncDeps[0] != "right" {
		t.Fatalf("after AsyncDeps = %#v, want right", after.AsyncDeps)
	}
}

func TestCompileParallelRejectsPeerRefsAndDuplicateIDs(t *testing.T) {
	cases := []string{
		`parallel { step a { b.ok } step b { nil } }`,
		`step a { nil } parallel { step a { nil } }`,
		`parallel { step a { nil } } parallel { async step a { nil } }`,
	}
	for _, source := range cases {
		flow, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", source, err)
		}
		err = NewCompiler().CompileFlow(context.Background(), &flow, runtime.NewContainer(runtime.NewLogger(nil)))
		if err == nil {
			t.Fatalf("CompileFlow(%q) succeeded, want error", source)
		}
	}
}

func TestCompileParallelRejectsResponseAndUserNext(t *testing.T) {
	cases := []string{
		`parallel { step a { response.json({body: {}}) } }`,
		`parallel { step a(condition: response.json({body: {}})) { nil } }`,
		`step a { {__next: "b"} } step b { nil }`,
	}
	for _, source := range cases {
		flow, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", source, err)
		}
		err = NewCompiler().CompileFlow(context.Background(), &flow, runtime.NewContainer(runtime.NewLogger(nil)))
		if err == nil {
			t.Fatalf("CompileFlow(%q) succeeded, want error", source)
		}
		if !strings.Contains(err.Error(), "response") && !strings.Contains(err.Error(), "__next") {
			t.Fatalf("unexpected error for %q: %v", source, err)
		}
	}
}

func TestCompileAllowsNextTextThatIsNotReturnedControlKey(t *testing.T) {
	cases := []string{
		`step a { {message: "__next is reserved"} }`,
		`step a { {metadata: {"__next": "data value"}} }`,
		"step a { `SELECT '__next' AS marker` }",
	}
	for _, source := range cases {
		flow, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", source, err)
		}
		if err := NewCompiler().CompileFlow(context.Background(), &flow, runtime.NewContainer(runtime.NewLogger(nil))); err != nil {
			t.Fatalf("CompileFlow(%q) error = %v", source, err)
		}
	}
}
