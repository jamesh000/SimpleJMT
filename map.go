package main

import (
	"fmt"
	"maps"
	"sort"
)

type MemStore struct {
	nodes  map[memStoreKey]Node
	values map[KeyHash][]valueInstance
}

type memStoreKey struct {
	version    Version
	nibblePath string
}

type valueInstance struct {
	version uint64
	val     Value
}

func (store MemStore) GetNode(key NodeKey) (Node, error) {
	node, ok := store.nodes[memStoreKey{
		key.version,
		string(key.nibblePath),
	}]
	if !ok {
		return nil, fmt.Errorf("Node %v:%v not found", key.version, key.nibblePath)
	}

	return node, nil
}

func (store MemStore) GetValue(key KeyHash, maxVersion uint64) (Value, error) {
	values, ok := store.values[key]
	if !ok {
		return nil, fmt.Errorf("There is no value for key hash %v", key)
	}

	i := sort.Search(len(values), func(i int) bool {
		return values[i].version > maxVersion
	})

	if i > 0 {
		return values[i-1].val, nil
	}

	return nil, fmt.Errorf("There is no value for key hash %v before or at version %v", key, maxVersion)
}

func (store MemStore) WriteNodeBatch(batch map[NodeKey]Node) error {
	maps.Copy(store.nodes, batch)

	return nil
}
