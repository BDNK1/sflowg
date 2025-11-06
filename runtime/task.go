package runtime

type Task interface {
	Execute(*Execution, map[string]any) (map[string]any, error)
}
