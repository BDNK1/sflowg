package runtime

// SubflowInvoker is implemented by the application runtime so DSL globals can
// call another loaded flow without depending on App directly.
type SubflowInvoker interface {
	LookupFlow(name string) (*Flow, bool)
	InvokeSubflow(parent *Execution, target *Flow, args map[string]any) (map[string]any, error)
}

// FlowSetValidator performs startup validation over the fully loaded flow set.
type FlowSetValidator interface {
	ValidateFlows(flows map[string]Flow) error
}
