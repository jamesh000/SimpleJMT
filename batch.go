package jmt

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

type keyValue struct {
	key       Hash
	valueHash Hash
}

type nibbleGroup struct {
	nibble byte
	kvs    []keyValue
}

// BatchUpdate applies all updates at newVersion as one atomic node batch. Keys
// are deduplicated by the map input and sorted internally so the result is
// independent of map iteration order.
func (jmt JellyfishMerkleTree) BatchUpdate(oldVersion Version, newVersion Version, updates map[Hash]Hash) (*Hash, error) {
	if jmt.writer == nil {
		return nil, fmt.Errorf("JellyfishMerkleTree has no writer")
	}

	kvs := make([]keyValue, 0, len(updates))
	for key, valueHash := range updates {
		kvs = append(kvs, keyValue{key: key, valueHash: valueHash})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return bytes.Compare(kvs[i].key[:], kvs[j].key[:]) < 0
	})

	nodeBatch := make(map[NodeKey]Node)
	rootKey := NodeKey{Version: oldVersion}

	oldRoot, err := jmt.reader.GetNode(rootKey)
	if err != nil {
		if !errors.Is(err, ErrNodeNotFound) {
			return nil, err
		}

		if len(kvs) == 0 {
			rootHash := EmptyRootHash
			return &rootHash, nil
		}

		oldRoot, err = jmt.batchUpdateSubtree(
			NodeKey{Version: newVersion},
			newVersion,
			kvs,
			0,
			nodeBatch,
		)
		if err != nil {
			return nil, err
		}
	} else if len(kvs) > 0 {
		oldRoot, err = jmt.batchInsertAt(
			rootKey,
			newVersion,
			kvs,
			0,
			nodeBatch,
		)
		if err != nil {
			return nil, err
		}
	}

	if oldRoot == nil {
		return nil, fmt.Errorf("batch update produced no root node")
	}

	nodeBatch[NodeKey{Version: newVersion}] = oldRoot

	if err := jmt.writer.WriteNodeBatch(nodeBatch); err != nil {
		return nil, err
	}

	rootHash := oldRoot.Hash()
	return &rootHash, nil
}

// batchInsertAt updates an existing subtree rooted at currentKey.
func (jmt JellyfishMerkleTree) batchInsertAt(
	currentKey NodeKey,
	newVersion Version,
	kvs []keyValue,
	depth int,
	nodeBatch map[NodeKey]Node,
) (Node, error) {
	node, err := jmt.reader.GetNode(currentKey)
	if err != nil {
		return nil, err
	}

	switch n := node.(type) {
	case *InternalNode:
		newNode := n.Copy()

		groups, err := groupByNibble(kvs, depth)
		if err != nil {
			return nil, err
		}

		for _, group := range groups {
			childPath, err := currentKey.NibblePath.Append(group.nibble)
			if err != nil {
				return nil, err
			}

			var childNode Node
			oldChild, ok := n.Children[group.nibble]
			if ok {
				oldChildKey := NodeKey{
					Version:    oldChild.version,
					NibblePath: childPath,
				}
				childNode, err = jmt.batchInsertAt(oldChildKey, newVersion, group.kvs, depth+1, nodeBatch)
			} else {
				childNode, err = jmt.batchUpdateSubtree(
					NodeKey{Version: newVersion, NibblePath: childPath},
					newVersion,
					group.kvs,
					depth+1,
					nodeBatch,
				)
			}
			if err != nil {
				return nil, err
			}

			newNode.insertChild(group.nibble, newVersion, childNode.Hash(), nodeTypeOf(childNode))
			nodeBatch[NodeKey{Version: newVersion, NibblePath: childPath}] = childNode
		}

		return newNode, nil

	case *LeafNode:
		return jmt.batchUpdateSubtreeWithExistingLeaf(
			currentKey,
			n,
			newVersion,
			kvs,
			depth,
			nodeBatch,
		)
	}

	return nil, fmt.Errorf("invalid node type")
}

// batchUpdateSubtree builds a subtree in an empty region of the tree.
func (jmt JellyfishMerkleTree) batchUpdateSubtree(
	nodeKey NodeKey,
	newVersion Version,
	kvs []keyValue,
	depth int,
	nodeBatch map[NodeKey]Node,
) (Node, error) {
	if len(kvs) == 1 {
		kv := kvs[0]
		return NewLeafNode(
			kv.key,
			kv.valueHash,
			ValueIndex{KeyHash: kv.key, Version: newVersion},
		), nil
	}

	groups, err := groupByNibble(kvs, depth)
	if err != nil {
		return nil, err
	}

	newNode := NewInternalNode()
	for _, group := range groups {
		childPath, err := nodeKey.NibblePath.Append(group.nibble)
		if err != nil {
			return nil, err
		}
		childKey := NodeKey{Version: newVersion, NibblePath: childPath}

		childNode, err := jmt.batchUpdateSubtree(childKey, newVersion, group.kvs, depth+1, nodeBatch)
		if err != nil {
			return nil, err
		}

		newNode.insertChild(group.nibble, newVersion, childNode.Hash(), nodeTypeOf(childNode))
		nodeBatch[childKey] = childNode
	}

	return newNode, nil
}

// batchUpdateSubtreeWithExistingLeaf updates a subtree where an existing leaf
// occupies the current path.
func (jmt JellyfishMerkleTree) batchUpdateSubtreeWithExistingLeaf(
	currentKey NodeKey,
	existingLeaf *LeafNode,
	newVersion Version,
	kvs []keyValue,
	depth int,
	nodeBatch map[NodeKey]Node,
) (Node, error) {
	if len(kvs) == 1 && kvs[0].key == existingLeaf.keyHash {
		kv := kvs[0]
		return NewLeafNode(
			existingLeaf.keyHash,
			kv.valueHash,
			ValueIndex{KeyHash: existingLeaf.keyHash, Version: newVersion},
		), nil
	}

	groups, err := groupByNibble(kvs, depth)
	if err != nil {
		return nil, err
	}

	existingNibble := nibbleAt(existingLeaf.keyHash, depth)
	existingHandled := false
	newNode := NewInternalNode()

	for _, group := range groups {
		childPath, err := currentKey.NibblePath.Append(group.nibble)
		if err != nil {
			return nil, err
		}
		childKey := NodeKey{Version: newVersion, NibblePath: childPath}

		var childNode Node
		if group.nibble == existingNibble {
			existingHandled = true
			childNode, err = jmt.batchUpdateSubtreeWithExistingLeaf(
				childKey,
				existingLeaf,
				newVersion,
				group.kvs,
				depth+1,
				nodeBatch,
			)
		} else {
			childNode, err = jmt.batchUpdateSubtree(
				childKey,
				newVersion,
				group.kvs,
				depth+1,
				nodeBatch,
			)
		}
		if err != nil {
			return nil, err
		}

		newNode.insertChild(group.nibble, newVersion, childNode.Hash(), nodeTypeOf(childNode))
		nodeBatch[childKey] = childNode
	}

	if !existingHandled {
		childPath, err := currentKey.NibblePath.Append(existingNibble)
		if err != nil {
			return nil, err
		}

		newNode.insertChild(existingNibble, newVersion, existingLeaf.Hash(), leafType)
		nodeBatch[NodeKey{Version: newVersion, NibblePath: childPath}] = existingLeaf
	}

	return newNode, nil
}

func groupByNibble(kvs []keyValue, depth int) ([]nibbleGroup, error) {
	if depth >= HashLen*2 {
		return nil, fmt.Errorf("cannot group keys at depth %d", depth)
	}

	groups := make([]nibbleGroup, 0, 16)
	for _, kv := range kvs {
		nibble := nibbleAt(kv.key, depth)
		if len(groups) == 0 || groups[len(groups)-1].nibble != nibble {
			groups = append(groups, nibbleGroup{nibble: nibble})
		}
		last := len(groups) - 1
		groups[last].kvs = append(groups[last].kvs, kv)
	}

	return groups, nil
}

func nibbleAt(key Hash, depth int) byte {
	if depth%2 == 0 {
		return key[depth/2] >> 4
	}
	return key[depth/2] & 0x0F
}

func nodeTypeOf(node Node) nodeType {
	switch node.(type) {
	case *LeafNode:
		return leafType
	case *InternalNode:
		return internalType
	default:
		panic(fmt.Errorf("invalid node type %T", node))
	}
}
