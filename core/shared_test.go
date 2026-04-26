package runtime

type noopEvaluator struct{}

func (noopEvaluator) Eval(execution *Execution, expression string) (any, error) {
	return nil, nil
}

func (noopEvaluator) EvalWithEnv(execution *Execution, expression string, extraVars map[string]any) (any, error) {
	return nil, nil
}

type testValueStore struct {
	values map[string]any
}

func newTestValueStore() ValueStore {
	return &testValueStore{values: make(map[string]any)}
}

func (s *testValueStore) Set(key string, value any) {
	s.values[key] = value
}

func (s *testValueStore) Get(key string) (any, bool) {
	value, ok := s.values[key]
	return value, ok
}

func (s *testValueStore) SetNested(prefix string, value any) {
	s.values[prefix] = value
}

func (s *testValueStore) Snapshot() map[string]any {
	out := make(map[string]any, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}
