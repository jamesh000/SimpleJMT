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
	return jmt.BatchUpdate(oldVersion, newVersion, map[Hash]Hash{key: valueHash})
}
