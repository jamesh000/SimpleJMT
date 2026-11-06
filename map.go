package jmt

import (
	"fmt"
	"maps"
)

type MemStore struct {
	nodes  map[NodeKey]Node
	values map[ValueKey]Value
}

func NewMemStore() *MemStore {
	return &MemStore{
		nodes:  make(map[NodeKey]Node),
		values: make(map[ValueKey]Value),
	}
}

func (store *MemStore) GetNode(key NodeKey) (Node, error) {
	node, ok := store.nodes[key]
	if !ok {
		return nil, fmt.Errorf("%w: version %d path %s", ErrNodeNotFound, key.Version, key.NibblePath.String())
	}

	return node, nil
}

// WriteValue stores a value at an exact value key. A nil value is a tombstone.
func (store *MemStore) WriteValue(key ValueKey, value Value) error {
	store.values[key] = value
	return nil
}

// GetValue returns the newest value for key at or before maxVersion. A nil
// value means the newest entry is a tombstone.
func (store *MemStore) GetValue(key Hash, maxVersion Version) (Value, error) {
	var best ValueKey
	found := false

	for valueKey := range store.values {
		if valueKey.KeyHash != key || valueKey.Version > maxVersion {
			continue
		}
		if !found || valueKey.Version > best.Version {
			best = valueKey
			found = true
		}
	}

	if !found {
		return nil, fmt.Errorf("There is no value for key hash %v before or at version %v", key, maxVersion)
	}

	return store.values[best], nil
}

func (store *MemStore) WriteNodeBatch(batch map[NodeKey]Node) error {
	maps.Copy(store.nodes, batch)

	return nil
}
