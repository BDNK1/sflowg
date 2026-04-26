package runtime

import (
	"strings"
	"sync"
)

// MapValueStore stores execution state as nested maps.
// It preserves hierarchy so DSL engines can expose native dot-access over values.
type MapValueStore struct {
	mu     sync.RWMutex
	values map[string]any
}

func NewValueStore() *MapValueStore {
	return &MapValueStore{
		values: make(map[string]any),
	}
}

// Set stores a value at a dot-separated key path, creating intermediate maps.
func (s *MapValueStore) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		s.values[key] = value
		return
	}

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
			m := make(map[string]any)
			current[part] = m
			current = m
		}
	}
	current[parts[len(parts)-1]] = value
}

// Get retrieves a value at a dot-separated key path by traversing nested maps.
func (s *MapValueStore) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
func (s *MapValueStore) SetNested(prefix string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setNestedLocked(prefix, value)
}

func (s *MapValueStore) setNestedLocked(prefix string, value any) {
	s.setLocked(prefix, value)

	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			s.setNestedLocked(prefix+"."+k, val)
		}
	case []any:
		// Arrays are stored as the slice value at the prefix key.
		// Individual elements are not expanded to numbered keys
		// since Risor supports native list indexing.
	default:
	}
}

func (s *MapValueStore) setLocked(key string, value any) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		s.values[key] = value
		return
	}

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
			m := make(map[string]any)
			current[part] = m
			current = m
		}
	}
	current[parts[len(parts)-1]] = value
}

// Snapshot returns a deep copy of the store contents.
func (s *MapValueStore) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.values)
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneSlice(src []any) []any {
	if src == nil {
		return nil
	}
	out := make([]any, len(src))
	for i, v := range src {
		out[i] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneMap(x)
	case []any:
		return cloneSlice(x)
	default:
		return x
	}
}
