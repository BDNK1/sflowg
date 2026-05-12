package runtime

type noopEvaluator struct{}

func (noopEvaluator) EvalExpression(execution *Execution, expr string, program ExpressionProgram, storeKeys []string, extra map[string]any) (any, error) {
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

func (s *testValueStore) SnapshotKeys(keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out
}
