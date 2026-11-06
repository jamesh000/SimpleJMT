package jmt

import (
	"fmt"
)

type Version = uint64

type JellyfishMerkleTree struct {
	reader TreeReader
	writer TreeWriter
}

func NewJellyfishMerkleTree(reader TreeReader, writer TreeWriter) *JellyfishMerkleTree {
	return &JellyfishMerkleTree{
		reader: reader,
		writer: writer,
	}
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

func (jmt JellyfishMerkleTree) Update(oldVersion Version, newVersion Version, key Hash, valueHash Hash) (*Hash, error) {
	if jmt.writer == nil {
		return nil, fmt.Errorf("JellyfishMerkleTree has no writer")
	}

	currentKey := NodeKey{Version: oldVersion}
	nodeBatch := make(map[NodeKey]Node)

	_, err := jmt.reader.GetNode(currentKey)
	if err != nil {
		leafRoot := &LeafNode{
			keyHash:   key,
			valueHash: valueHash,
		}
		nodeBatch[NodeKey{Version: newVersion}] = leafRoot

		if err := jmt.writer.WriteNodeBatch(nodeBatch); err != nil {
			return nil, err
		}

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
	if err != nil {
		return nil, err
	}

	if err := jmt.writer.WriteNodeBatch(nodeBatch); err != nil {
		return nil, err
	}

	rootHash := rootChild.hash

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

	switch node := readNode.(type) {
	case *InternalNode:
		n, err := keyPath.getNibble(pos)
		if err != nil {
			return nil, err
		}

		newNode := node.Copy()
		var newChild *child

		c, ok := newNode.Children[n]
		if !ok {
			// create and add the leaf to the batch
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
		newNode.insertChild(n, newChild.version, newChild.hash, newChild.nodeType)
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
		// Same key: this is just a newer version of the existing leaf.
		if node.keyHash == key {
			newLeaf := &LeafNode{
				keyHash:   key,
				valueHash: valueHash,
			}
			nodeBatch[NodeKey{
				Version:    newVersion,
				NibblePath: currentKey.NibblePath,
			}] = newLeaf

			return &child{
				version:  newVersion,
				hash:     newLeaf.Hash(),
				nodeType: leafType,
			}, nil
		}

		// Different keys: the leaf and the new key share the prefix up to
		// currentKey.NibblePath. Add internal nodes until the keys diverge so
		// the two leaves can be told apart.
		existingKeyPath, err := NewNibblePath(HashLen*2, node.keyHash[:])
		if err != nil {
			return nil, err
		}

		splitIndex := pos
		for splitIndex < HashLen*2 {
			existingNibble, err := existingKeyPath.getNibble(splitIndex)
			if err != nil {
				return nil, err
			}
			newNibble, err := keyPath.getNibble(splitIndex)
			if err != nil {
				return nil, err
			}
			if existingNibble != newNibble {
				break
			}
			splitIndex++
		}
		if splitIndex >= HashLen*2 {
			return nil, fmt.Errorf("keys differ but share the entire nibble path")
		}

		existingNibble, err := existingKeyPath.getNibble(splitIndex)
		if err != nil {
			return nil, err
		}
		newNibble, err := keyPath.getNibble(splitIndex)
		if err != nil {
			return nil, err
		}

		// Leaves live one nibble below the internal node that splits them.
		existingLeafPath, err := existingKeyPath.Prefix(splitIndex + 1)
		if err != nil {
			return nil, err
		}
		newLeafPath, err := keyPath.Prefix(splitIndex + 1)
		if err != nil {
			return nil, err
		}

		newLeaf := &LeafNode{
			keyHash:   key,
			valueHash: valueHash,
		}

		nodeBatch[NodeKey{
			Version:    currentKey.Version,
			NibblePath: *existingLeafPath,
		}] = node
		nodeBatch[NodeKey{
			Version:    newVersion,
			NibblePath: *newLeafPath,
		}] = newLeaf

		splitNode := NewInternalNode()
		splitNode.insertChild(existingNibble, currentKey.Version, node.Hash(), leafType)
		splitNode.insertChild(newNibble, newVersion, newLeaf.Hash(), leafType)

		splitPath, err := keyPath.Prefix(splitIndex)
		if err != nil {
			return nil, err
		}
		nodeBatch[NodeKey{
			Version:    newVersion,
			NibblePath: *splitPath,
		}] = splitNode

		currentChild := &child{
			version:  newVersion,
			hash:     splitNode.Hash(),
			nodeType: internalType,
		}

		// Chain single-child internal nodes back up to the position of the
		// original leaf.
		for depth := splitIndex - 1; depth >= pos; depth-- {
			nibble, err := keyPath.getNibble(depth)
			if err != nil {
				return nil, err
			}

			parent := NewInternalNode()
			parent.insertChild(nibble, currentChild.version, currentChild.hash, currentChild.nodeType)

			parentPath, err := keyPath.Prefix(depth)
			if err != nil {
				return nil, err
			}
			nodeBatch[NodeKey{
				Version:    newVersion,
				NibblePath: *parentPath,
			}] = parent

			currentChild = &child{
				version:  newVersion,
				hash:     parent.Hash(),
				nodeType: internalType,
			}
		}

		return currentChild, nil
	}

	return nil, fmt.Errorf("Invalid node type")
}
