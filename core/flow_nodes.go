package runtime

// NormalizeFlowNodes returns the canonical top-level node sequence for a flow.
// Flow.Steps remains accepted as compatibility input during the node migration.
func NormalizeFlowNodes(flow *Flow) []FlowNode {
	if flow == nil {
		return nil
	}
	if len(flow.Nodes) > 0 {
		return flow.Nodes
	}
	nodes := make([]FlowNode, 0, len(flow.Steps))
	for i := range flow.Steps {
		step := flow.Steps[i]
		nodes = append(nodes, FlowNode{
			ID:   step.ID,
			Kind: FlowNodeStep,
			Step: &step,
		})
	}
	return nodes
}
