package graph

import (
	"fmt"
	"strings"

	"github.com/sflowg/sflowg/cli/internal/analyzer"
)

// Graph represents a dependency graph between plugins
type Graph struct {
	// nodes maps plugin name to its metadata
	nodes map[string]*analyzer.PluginMetadata

	// edges maps plugin name to list of plugins it depends on
	edges map[string][]string

	// reverseEdges maps plugin name to list of plugins that depend on it
	reverseEdges map[string][]string
}

// BuildGraph constructs a dependency graph from plugin metadata
func BuildGraph(plugins []*analyzer.PluginMetadata) (*Graph, error) {
	g := &Graph{
		nodes:        make(map[string]*analyzer.PluginMetadata),
		edges:        make(map[string][]string),
		reverseEdges: make(map[string][]string),
	}

	// First pass: register all nodes
	for _, plugin := range plugins {
		g.nodes[plugin.Name] = plugin
		g.edges[plugin.Name] = []string{}
		g.reverseEdges[plugin.Name] = []string{}
	}

	// Second pass: build edges
	for _, plugin := range plugins {
		for _, dep := range plugin.Dependencies {
			// Check if dependency exists
			if _, exists := g.nodes[dep.PluginName]; !exists {
				return nil, &GraphError{
					Type:       ErrorMissingDependency,
					PluginName: plugin.Name,
					Message:    fmt.Sprintf("dependency '%s' required but not registered", dep.PluginName),
					Details: map[string]string{
						"field":      dep.FieldName,
						"dependency": dep.PluginName,
					},
				}
			}

			// Add edge: plugin -> dependency
			g.edges[plugin.Name] = append(g.edges[plugin.Name], dep.PluginName)

			// Add reverse edge: dependency <- plugin
			g.reverseEdges[dep.PluginName] = append(g.reverseEdges[dep.PluginName], plugin.Name)
		}
	}

	// Validate graph (check for cycles)
	if cycle := g.findCycle(); cycle != nil {
		return nil, &GraphError{
			Type:       ErrorCircularDependency,
			PluginName: cycle[0],
			Message:    fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " → ")),
			Details: map[string]string{
				"cycle": strings.Join(cycle, " → "),
			},
		}
	}

	return g, nil
}

// TopologicalSort returns plugins in dependency order (dependencies first)
// Uses Kahn's algorithm for topological sorting
func (g *Graph) TopologicalSort() ([]string, error) {
	// Calculate in-degrees (number of dependencies)
	inDegree := make(map[string]int)
	for node := range g.nodes {
		inDegree[node] = len(g.edges[node])
	}

	// Queue of nodes with no dependencies
	queue := []string{}
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	// Result in topological order
	result := []string{}

	// Process queue
	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// For each plugin that depends on current
		for _, dependent := range g.reverseEdges[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// If not all nodes processed, there's a cycle
	if len(result) != len(g.nodes) {
		cycle := g.findCycle()
		return nil, &GraphError{
			Type:       ErrorCircularDependency,
			PluginName: cycle[0],
			Message:    fmt.Sprintf("circular dependency prevents ordering: %s", strings.Join(cycle, " → ")),
			Details: map[string]string{
				"cycle": strings.Join(cycle, " → "),
			},
		}
	}

	return result, nil
}

// findCycle detects and returns a cycle in the graph, or nil if no cycle exists
// Uses DFS with recursion stack tracking
func (g *Graph) findCycle() []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	var dfs func(node string) []string

	dfs = func(node string) []string {
		visited[node] = true
		recStack[node] = true

		for _, dep := range g.edges[node] {
			if !visited[dep] {
				parent[dep] = node
				if cycle := dfs(dep); cycle != nil {
					return cycle
				}
			} else if recStack[dep] {
				// Found cycle: reconstruct it
				cycle := []string{dep}
				current := node
				for current != dep {
					cycle = append([]string{current}, cycle...)
					current = parent[current]
				}
				cycle = append(cycle, dep) // Complete the cycle
				return cycle
			}
		}

		recStack[node] = false
		return nil
	}

	for node := range g.nodes {
		if !visited[node] {
			if cycle := dfs(node); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// HasCycle returns true if the graph contains a circular dependency
func (g *Graph) HasCycle() bool {
	return g.findCycle() != nil
}

// GetDependencies returns the list of dependencies for a given plugin
func (g *Graph) GetDependencies(pluginName string) []string {
	return g.edges[pluginName]
}

// GetDependents returns the list of plugins that depend on the given plugin
func (g *Graph) GetDependents(pluginName string) []string {
	return g.reverseEdges[pluginName]
}

// Nodes returns all plugin names in the graph
func (g *Graph) Nodes() []string {
	nodes := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		nodes = append(nodes, name)
	}
	return nodes
}

// GetMetadata returns the plugin metadata for a given plugin name
func (g *Graph) GetMetadata(pluginName string) *analyzer.PluginMetadata {
	return g.nodes[pluginName]
}

// GraphError represents errors that occur during graph operations
type GraphError struct {
	Type       ErrorType
	PluginName string
	Message    string
	Details    map[string]string
}

func (e *GraphError) Error() string {
	return e.Message
}

// ErrorType represents different types of graph errors
type ErrorType int

const (
	ErrorMissingDependency ErrorType = iota
	ErrorCircularDependency
	ErrorInvalidGraph
)

func (t ErrorType) String() string {
	switch t {
	case ErrorMissingDependency:
		return "MissingDependency"
	case ErrorCircularDependency:
		return "CircularDependency"
	case ErrorInvalidGraph:
		return "InvalidGraph"
	default:
		return "Unknown"
	}
}
