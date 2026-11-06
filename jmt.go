package jmt

import (
	"errors"
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

// EmptyRootHash is the root hash of an empty tree.
var EmptyRootHash = emptyPlaceholderHash

// Root returns the root hash of the tree at version. A version with no stored
// root node is treated as the empty tree.
func (jmt JellyfishMerkleTree) Root(version Version) (Hash, error) {
	root, err := jmt.reader.GetNode(NodeKey{Version: version})
	if errors.Is(err, ErrNodeNotFound) {
		return EmptyRootHash, nil
	}
	if err != nil {
		return Hash{}, err
	}

	return root.Hash(), nil
}

func (jmt JellyfishMerkleTree) Lookup(version Version, key Hash) (*Hash, error) {
	leaf, err := jmt.lookupLeaf(version, key)
	if err != nil || leaf == nil {
		return nil, err
	}

	valueHash := leaf.valueHash
	return &valueHash, nil
}

// LookupWithIndex returns the value hash and the index of the value in the
// separate versioned value store.
func (jmt JellyfishMerkleTree) LookupWithIndex(version Version, key Hash) (*Hash, *ValueIndex, error) {
	leaf, err := jmt.lookupLeaf(version, key)
	if err != nil || leaf == nil {
		return nil, nil, err
	}

	valueHash := leaf.valueHash
	valueIndex := leaf.valueIndex
	return &valueHash, &valueIndex, nil
}

func (jmt JellyfishMerkleTree) lookupLeaf(version Version, key Hash) (*LeafNode, error) {
	// root nodes only have version, their nibble path is empty (zeroed)
	currentKey := NodeKey{Version: version} // start at root

	keyPath, err := NewNibblePath(len(key)*2, key[:])
	if err != nil {
		return nil, err
	}

	// A leaf can live at depth HashLen*2 when keys differ only in the last
	// nibble, so visit depths 0 through HashLen*2 inclusive.
	for depth := 0; depth <= HashLen*2; depth++ {
		node, err := jmt.reader.GetNode(currentKey)
		if err != nil {
			// A missing root is the empty tree. A missing non-root node means
			// the tree is corrupt or the storage layer failed.
			if errors.Is(err, ErrNodeNotFound) && currentKey.NibblePath.length == 0 {
				return nil, nil
			}
			return nil, err
		}

		switch n := node.(type) {
		case *InternalNode:
			nibble, err := keyPath.getNibble(depth)
			if err != nil {
				return nil, err
			}

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
				return n, nil
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
		if !errors.Is(err, ErrNodeNotFound) {
			return nil, err
		}

		leafRoot := NewLeafNode(key, valueHash, ValueIndex{KeyHash: key, Version: newVersion})
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
			newLeaf := NewLeafNode(key, valueHash, ValueIndex{KeyHash: key, Version: newVersion})
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
			newLeaf := NewLeafNode(key, valueHash, ValueIndex{KeyHash: key, Version: newVersion})
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

		newLeaf := NewLeafNode(key, valueHash, ValueIndex{KeyHash: key, Version: newVersion})

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
