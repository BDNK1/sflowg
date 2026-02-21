package yaml

import "fmt"

// ValueStore stores values in a flat map with underscore-separated keys.
// Keys like "step.result.field" are stored as "step_result_field".
// Nested maps and arrays are recursively expanded into flat keys.
type ValueStore struct {
	values map[string]any
}

func NewValueStore() *ValueStore {
	return &ValueStore{
		values: make(map[string]any),
	}
}

func (s *ValueStore) Set(key string, value any) {
	s.values[FormatKey(key)] = value
}

func (s *ValueStore) Get(key string) (any, bool) {
	v, ok := s.values[FormatKey(key)]
	return v, ok
}

// SetNested stores a value and recursively expands nested maps/arrays into flat keys.
// This enables accessing nested results like step.result.body.id as individual keys.
func (s *ValueStore) SetNested(prefix string, value any) {
	s.Set(prefix, value)

	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			s.SetNested(prefix+"."+k, v)
		}
	}

	if arr, ok := value.([]any); ok {
		for i, v := range arr {
			s.SetNested(fmt.Sprintf("%s.%d", prefix, i), v)
		}
	}
}

func (s *ValueStore) All() map[string]any {
	return s.values
}
