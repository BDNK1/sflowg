package runtime

import (
	"strings"
	"sync"
)

const maxValueStoreDepth = 64

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
		s.values[key] = cloneValue(value, maxValueStoreDepth)
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
	current[parts[len(parts)-1]] = cloneValue(value, maxValueStoreDepth)
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
	s.setNestedLocked(prefix, cloneValue(value, maxValueStoreDepth), maxValueStoreDepth)
}

func (s *MapValueStore) setNestedLocked(prefix string, value any, depth int) {
	s.setLocked(prefix, value)
	if depth <= 0 {
		return
	}

	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			s.setNestedLocked(prefix+"."+k, val, depth-1)
		}
	case []any:
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
	return cloneMap(s.values, maxValueStoreDepth)
}

func (s *MapValueStore) SnapshotKeys(keys []string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := getFromMap(s.values, key); ok {
			out[key] = cloneValue(value, maxValueStoreDepth)
		}
	}
	return out
}

type ReadOnlyValueView interface {
	Get(key string) (any, bool)
	SnapshotKeys(keys []string) map[string]any
}

type readOnlyValueView struct {
	values map[string]any
}

func NewReadOnlyValueView(store ValueStore, keys []string) ReadOnlyValueView {
	if store == nil {
		return &readOnlyValueView{values: make(map[string]any)}
	}
	var snapshot map[string]any
	if keys == nil {
		snapshot = store.Snapshot()
	} else {
		bounded := store.SnapshotKeys(keys)
		normalized := NewValueStore()
		for key, value := range bounded {
			normalized.SetNested(key, value)
		}
		snapshot = normalized.Snapshot()
	}
	return &readOnlyValueView{values: snapshot}
}

func (v *readOnlyValueView) Get(key string) (any, bool) {
	value, ok := getFromMap(v.values, key)
	if !ok {
		return nil, false
	}
	return cloneValue(value, maxValueStoreDepth), true
}

func (v *readOnlyValueView) SnapshotKeys(keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := getFromMap(v.values, key); ok {
			out[key] = cloneValue(value, maxValueStoreDepth)
		}
	}
	return out
}

func (v *readOnlyValueView) snapshot() map[string]any {
	return cloneMap(v.values, maxValueStoreDepth)
}

type ScopedValueStore struct {
	parent ReadOnlyValueView
	local  ValueStore
}

func NewScopedValueStore(parent ReadOnlyValueView) *ScopedValueStore {
	return &ScopedValueStore{
		parent: parent,
		local:  NewValueStore(),
	}
}

func (s *ScopedValueStore) Set(key string, value any) {
	s.local.Set(key, value)
}

func (s *ScopedValueStore) Get(key string) (any, bool) {
	if value, ok := s.local.Get(key); ok {
		return value, true
	}
	if s.parent == nil {
		return nil, false
	}
	return s.parent.Get(key)
}

func (s *ScopedValueStore) SetNested(prefix string, value any) {
	s.local.SetNested(prefix, value)
}

func (s *ScopedValueStore) Snapshot() map[string]any {
	out := make(map[string]any)
	if parent, ok := s.parent.(interface{ snapshot() map[string]any }); ok {
		out = parent.snapshot()
	}
	mergeSnapshot(out, s.local.Snapshot())
	return out
}

func (s *ScopedValueStore) SnapshotKeys(keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	local := s.local.SnapshotKeys(keys)
	for _, key := range keys {
		if value, ok := local[key]; ok {
			out[key] = value
			continue
		}
		if s.parent == nil {
			continue
		}
		if value, ok := s.parent.Get(key); ok {
			out[key] = value
		}
	}
	return out
}

func getFromMap(values map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		v, ok := values[key]
		return v, ok
	}

	current := values
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

func mergeSnapshot(dst map[string]any, src map[string]any) {
	for key, value := range src {
		srcMap, srcIsMap := value.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			mergeSnapshot(dstMap, srcMap)
			continue
		}
		dst[key] = cloneValue(value, maxValueStoreDepth)
	}
}

func cloneMap(src map[string]any, depth int) map[string]any {
	if src == nil {
		return nil
	}
	if depth <= 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = cloneValue(v, depth-1)
	}
	return out
}

func cloneSlice(src []any, depth int) []any {
	if src == nil {
		return nil
	}
	if depth <= 0 {
		return []any{}
	}
	out := make([]any, len(src))
	for i, v := range src {
		out[i] = cloneValue(v, depth-1)
	}
	return out
}

func cloneValue(v any, depth int) any {
	if depth <= 0 {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		return cloneMap(x, depth)
	case []any:
		return cloneSlice(x, depth)
	default:
		return x
	}
}
