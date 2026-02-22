package dsl

import "strings"

// ValueStore stores execution state as nested maps, enabling native Risor
// dot-access on variables (e.g., step_name.body.id).
// Unlike the YAML ValueStore which flattens keys with underscores,
// this store preserves the hierarchy so Risor can traverse it naturally.
type ValueStore struct {
	values map[string]any
}

func NewValueStore() *ValueStore {
	return &ValueStore{
		values: make(map[string]any),
	}
}

// Set stores a value at a dot-separated key path, creating intermediate maps.
// Set("step.result.body", data) creates values["step"]["result"]["body"] = data.
func (s *ValueStore) Set(key string, value any) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		s.values[key] = value
		return
	}

	// Navigate/create intermediate maps
	current := s.values
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			m := make(map[string]any)
			current[part] = m
			current = m
			continue
		}
		if m, ok := next.(map[string]any); ok {
			current = m
		} else {
			// Overwrite non-map value with a new map
			m := make(map[string]any)
			current[part] = m
			current = m
		}
	}
	current[parts[len(parts)-1]] = value
}

// Get retrieves a value at a dot-separated key path by traversing nested maps.
func (s *ValueStore) Get(key string) (any, bool) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		v, ok := s.values[key]
		return v, ok
	}

	current := s.values
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			return nil, false
		}
		m, ok := next.(map[string]any)
		if !ok {
			return nil, false
		}
		current = m
	}

	v, ok := current[parts[len(parts)-1]]
	return v, ok
}

// SetNested stores a value and recursively expands nested maps/slices
// so all levels are accessible via dot notation.
func (s *ValueStore) SetNested(prefix string, value any) {
	s.Set(prefix, value)

	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			s.SetNested(prefix+"."+k, val)
		}
	case []any:
		// Arrays are stored as the slice value at the prefix key.
		// Individual elements are not expanded to numbered keys
		// since Risor supports native list indexing.
	default:
		// Leaf value — already stored above
	}
}

// All returns the top-level map. Risor receives this as globals
// and traverses nested maps via attribute access.
func (s *ValueStore) All() map[string]any {
	return s.values
}
