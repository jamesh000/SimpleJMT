package jmt

import (
	"fmt"
)

type Version = uint64

type JellyfishMerkleTree struct {
	reader TreeReader
}

var emptyPlaceholderHash = Hash{
	0xd5, 0xa7, 0x97, 0x07,
	0xc0, 0xbb, 0x52, 0x2e,
	0x0d, 0xe8, 0x14, 0xa4,
	0x2f, 0xc8, 0x0b, 0xaa,
	0x78, 0xe3, 0xbe, 0xc1,
	0x26, 0x77, 0x20, 0x41,
	0x8c, 0x8c, 0x4c, 0x01,
	0x9d, 0x53, 0xa1, 0xfa,
}

func (jmt JellyfishMerkleTree) Lookup(version Version, key Hash) (*Hash, error) {
	// root nodes only have version, their nibble path is empty (zeroed)
	currentKey := NodeKey{Version: version} // start at root

	keyPath, err := NewNibblePath(len(key)*2, key[:])
	if err != nil {
		return nil, err
	}

	for _, nibble := range keyPath.Iter() {
		node, err := jmt.reader.GetNode(currentKey)
		if err != nil {
			return nil, err
		}

		switch n := node.(type) {
		case *InternalNode:
			// get the next child in the path if it exists
			child, ok := n.Children[nibble]
			if !ok {
				return nil, nil
			}

			currentKey.Version = child.version
			currentKey.NibblePath, err = currentKey.NibblePath.Append(nibble)
			if err != nil {
				return nil, err
			}
		case *LeafNode:
			if n.keyHash == key {
				foundValueHash := n.valueHash

				return &foundValueHash, nil
			}

			return nil, nil
		}
	}

	return nil, fmt.Errorf("reached end of key without finding value, erroneous node somewhere in tree")
}

// work in progress
func (jmt JellyfishMerkleTree) Update(oldVersion Version, newVersion Version, key Hash, valueHash Hash) (*Hash, error) {
	currentKey := NodeKey{Version: oldVersion}
	nodeBatch := make(map[NodeKey]Node)

	_, err := jmt.reader.GetNode(currentKey)
	if err != nil {
		leafRoot := &LeafNode{
			keyHash:   key,
			valueHash: valueHash,
		}
		nodeBatch[currentKey] = leafRoot
		rootHash := leafRoot.Hash()

		return &rootHash, nil
	}

	keyPath, err := NewNibblePath(len(key)*2, key[:])
	if err != nil {
		return nil, err
	}

	// recurse to update the tree and get the root hash
	rootChild, err := jmt.recursiveUpdate(
		newVersion,
		key,
		valueHash,
		nodeBatch,
		keyPath,
		0,
		currentKey,
	)
	rootHash := rootChild.hash

	if err != nil {
		return nil, err
	}

	return &rootHash, nil
}

func (jmt JellyfishMerkleTree) recursiveUpdate(
	newVersion Version,
	key Hash,
	valueHash Hash,
	nodeBatch map[NodeKey]Node,
	keyPath *NibblePath,
	pos int,
	currentKey NodeKey,
) (*child, error) {
	readNode, err := jmt.reader.GetNode(currentKey)
	if err != nil {
		return nil, err
	}

	n := keyPath.nibbles[pos]

	switch node := readNode.(type) {
	case *InternalNode:
		newNode := node.Copy()
		var newChild *child

		c, ok := newNode.Children[n]
		if !ok {
			// create a add the leaf to the batch
			leafNibblePath, err := currentKey.NibblePath.Append(n)
			if err != nil {
				return nil, err
			}
			leafKey := NodeKey{
				newVersion,
				leafNibblePath,
			}
			newLeaf := &LeafNode{
				keyHash:   key,
				valueHash: valueHash,
			}
			nodeBatch[leafKey] = newLeaf

			newChild = &child{
				version:  newVersion,
				hash:     newLeaf.Hash(),
				nodeType: leafType,
			}
		} else {
			nextNibble, err := currentKey.NibblePath.Append(n)
			if err != nil {
				return nil, err
			}
			nextPath := NodeKey{
				Version:    c.version,
				NibblePath: nextNibble,
			}

			newChild, err = jmt.recursiveUpdate(newVersion, key, valueHash, nodeBatch, keyPath, pos+1, nextPath)
			if err != nil {
				return nil, err
			}
		}

		// Finalize the new node and add it to the batch
		newNode.Children[n] = *newChild
		newPath := NodeKey{
			Version:    newVersion,
			NibblePath: currentKey.NibblePath,
		}
		nodeBatch[newPath] = newNode

		// return the child information
		return &child{
			version:  newVersion,
			hash:     newNode.Hash(),
			nodeType: internalType,
		}, nil
	case *LeafNode:
		return nil, fmt.Errorf("Currently unimplemented")
	}

	return nil, fmt.Errorf("Invalid node type")
}
