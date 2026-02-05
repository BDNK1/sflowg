package graph

import (
	"strings"
	"testing"

	"github.com/BDNK1/sflowg/cli/internal/analyzer"
)

func TestBuildGraph_NoDependencies(t *testing.T) {
	plugins := []*analyzer.PluginMetadata{
		{Name: "http", Dependencies: []analyzer.Dependency{}},
		{Name: "redis", Dependencies: []analyzer.Dependency{}},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	if len(graph.Nodes()) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes()))
	}

	if graph.HasCycle() {
		t.Error("Expected no cycle in graph")
	}
}

func TestBuildGraph_LinearChain(t *testing.T) {
	// A → B → C
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "c", PluginName: "c"},
			},
		},
		{
			Name:         "c",
			Dependencies: []analyzer.Dependency{},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	// Verify dependencies
	aDeps := graph.GetDependencies("a")
	if len(aDeps) != 1 || aDeps[0] != "b" {
		t.Errorf("Expected a to depend on [b], got %v", aDeps)
	}

	bDeps := graph.GetDependencies("b")
	if len(bDeps) != 1 || bDeps[0] != "c" {
		t.Errorf("Expected b to depend on [c], got %v", bDeps)
	}

	cDeps := graph.GetDependencies("c")
	if len(cDeps) != 0 {
		t.Errorf("Expected c to have no dependencies, got %v", cDeps)
	}

	// Verify reverse dependencies
	cDependents := graph.GetDependents("c")
	if len(cDependents) != 1 || cDependents[0] != "b" {
		t.Errorf("Expected c to be depended on by [b], got %v", cDependents)
	}
}

func TestBuildGraph_DiamondDependency(t *testing.T) {
	// A depends on B and C, both B and C depend on D
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
				{FieldName: "c", PluginName: "c"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "d", PluginName: "d"},
			},
		},
		{
			Name: "c",
			Dependencies: []analyzer.Dependency{
				{FieldName: "d", PluginName: "d"},
			},
		},
		{
			Name:         "d",
			Dependencies: []analyzer.Dependency{},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	if graph.HasCycle() {
		t.Error("Expected no cycle in diamond dependency")
	}

	// D should have 2 dependents (B and C)
	dDependents := graph.GetDependents("d")
	if len(dDependents) != 2 {
		t.Errorf("Expected d to have 2 dependents, got %d: %v", len(dDependents), dDependents)
	}
}

func TestTopologicalSort_LinearChain(t *testing.T) {
	// A → B → C
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "c", PluginName: "c"},
			},
		},
		{
			Name:         "c",
			Dependencies: []analyzer.Dependency{},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// Expected order: c, b, a (dependencies first)
	if len(order) != 3 {
		t.Fatalf("Expected 3 plugins in order, got %d", len(order))
	}

	// C should come before B, B should come before A
	cIdx := indexOf(order, "c")
	bIdx := indexOf(order, "b")
	aIdx := indexOf(order, "a")

	if cIdx > bIdx {
		t.Errorf("Expected c before b, got order: %v", order)
	}
	if bIdx > aIdx {
		t.Errorf("Expected b before a, got order: %v", order)
	}
}

func TestTopologicalSort_DiamondDependency(t *testing.T) {
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
				{FieldName: "c", PluginName: "c"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "d", PluginName: "d"},
			},
		},
		{
			Name: "c",
			Dependencies: []analyzer.Dependency{
				{FieldName: "d", PluginName: "d"},
			},
		},
		{
			Name:         "d",
			Dependencies: []analyzer.Dependency{},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// Verify D comes first, A comes last
	dIdx := indexOf(order, "d")
	aIdx := indexOf(order, "a")

	if dIdx > aIdx {
		t.Errorf("Expected d before a, got order: %v", order)
	}

	// B and C should both come after D and before A
	bIdx := indexOf(order, "b")
	cIdx := indexOf(order, "c")

	if dIdx > bIdx || dIdx > cIdx {
		t.Errorf("Expected d before b and c, got order: %v", order)
	}
	if bIdx > aIdx || cIdx > aIdx {
		t.Errorf("Expected b and c before a, got order: %v", order)
	}
}

func TestBuildGraph_SimpleCycle(t *testing.T) {
	// A → B → A (simple 2-node cycle)
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "a", PluginName: "a"},
			},
		},
	}

	_, err := BuildGraph(plugins)
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}

	graphErr, ok := err.(*GraphError)
	if !ok {
		t.Fatalf("Expected GraphError, got %T", err)
	}

	if graphErr.Type != ErrorCircularDependency {
		t.Errorf("Expected ErrorCircularDependency, got %v", graphErr.Type)
	}

	// Check that cycle is mentioned in error
	if !strings.Contains(graphErr.Message, "circular dependency") {
		t.Errorf("Expected 'circular dependency' in error message, got: %s", graphErr.Message)
	}
}

func TestBuildGraph_ComplexCycle(t *testing.T) {
	// A → B → C → A (3-node cycle)
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
			},
		},
		{
			Name: "b",
			Dependencies: []analyzer.Dependency{
				{FieldName: "c", PluginName: "c"},
			},
		},
		{
			Name: "c",
			Dependencies: []analyzer.Dependency{
				{FieldName: "a", PluginName: "a"},
			},
		},
	}

	_, err := BuildGraph(plugins)
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}

	graphErr, ok := err.(*GraphError)
	if !ok {
		t.Fatalf("Expected GraphError, got %T", err)
	}

	if graphErr.Type != ErrorCircularDependency {
		t.Errorf("Expected ErrorCircularDependency, got %v", graphErr.Type)
	}
}

func TestBuildGraph_MissingDependency(t *testing.T) {
	// A depends on B, but B is not registered
	plugins := []*analyzer.PluginMetadata{
		{
			Name: "a",
			Dependencies: []analyzer.Dependency{
				{FieldName: "b", PluginName: "b"},
			},
		},
	}

	_, err := BuildGraph(plugins)
	if err == nil {
		t.Fatal("Expected error for missing dependency")
	}

	graphErr, ok := err.(*GraphError)
	if !ok {
		t.Fatalf("Expected GraphError, got %T", err)
	}

	if graphErr.Type != ErrorMissingDependency {
		t.Errorf("Expected ErrorMissingDependency, got %v", graphErr.Type)
	}

	if graphErr.PluginName != "a" {
		t.Errorf("Expected error on plugin 'a', got '%s'", graphErr.PluginName)
	}

	if !strings.Contains(graphErr.Message, "dependency 'b'") {
		t.Errorf("Expected dependency 'b' mentioned in error, got: %s", graphErr.Message)
	}
}

func TestHasCycle_NoCycle(t *testing.T) {
	plugins := []*analyzer.PluginMetadata{
		{Name: "http", Dependencies: []analyzer.Dependency{}},
		{
			Name: "payment",
			Dependencies: []analyzer.Dependency{
				{FieldName: "http", PluginName: "http"},
			},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	if graph.HasCycle() {
		t.Error("Expected no cycle")
	}
}

func TestGetMetadata(t *testing.T) {
	plugins := []*analyzer.PluginMetadata{
		{
			Name:       "http",
			TypeName:   "HTTPPlugin",
			ImportPath: "github.com/test/http",
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	metadata := graph.GetMetadata("http")
	if metadata == nil {
		t.Fatal("Expected metadata, got nil")
	}

	if metadata.TypeName != "HTTPPlugin" {
		t.Errorf("Expected TypeName='HTTPPlugin', got '%s'", metadata.TypeName)
	}

	if metadata.ImportPath != "github.com/test/http" {
		t.Errorf("Expected ImportPath='github.com/test/http', got '%s'", metadata.ImportPath)
	}
}

func TestTopologicalSort_ComplexGraph(t *testing.T) {
	// More complex dependency graph:
	// http (no deps)
	// redis (no deps)
	// cache -> redis
	// payment -> http, cache
	// order -> payment, redis

	plugins := []*analyzer.PluginMetadata{
		{Name: "http", Dependencies: []analyzer.Dependency{}},
		{Name: "redis", Dependencies: []analyzer.Dependency{}},
		{
			Name: "cache",
			Dependencies: []analyzer.Dependency{
				{FieldName: "redis", PluginName: "redis"},
			},
		},
		{
			Name: "payment",
			Dependencies: []analyzer.Dependency{
				{FieldName: "http", PluginName: "http"},
				{FieldName: "cache", PluginName: "cache"},
			},
		},
		{
			Name: "order",
			Dependencies: []analyzer.Dependency{
				{FieldName: "payment", PluginName: "payment"},
				{FieldName: "redis", PluginName: "redis"},
			},
		},
	}

	graph, err := BuildGraph(plugins)
	if err != nil {
		t.Fatalf("BuildGraph failed: %v", err)
	}

	order, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// Verify constraints
	httpIdx := indexOf(order, "http")
	redisIdx := indexOf(order, "redis")
	cacheIdx := indexOf(order, "cache")
	paymentIdx := indexOf(order, "payment")
	orderIdx := indexOf(order, "order")

	// redis must come before cache
	if redisIdx > cacheIdx {
		t.Errorf("Expected redis before cache, got order: %v", order)
	}

	// http and cache must come before payment
	if httpIdx > paymentIdx || cacheIdx > paymentIdx {
		t.Errorf("Expected http and cache before payment, got order: %v", order)
	}

	// payment must come before order
	if paymentIdx > orderIdx {
		t.Errorf("Expected payment before order, got order: %v", order)
	}
}

// Helper function to find index of element in slice
func indexOf(slice []string, value string) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return -1
}
