package jmt

import "fmt"

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
		case InternalNode:
			// get the next child in the path if it exists
			child, ok := n.Children[nibble]
			if !ok {
				return nil, nil
			}

			currentKey.Version = child.version
			currentKey.NibblePath.Append(nibble)
		case LeafNode:
			if n.keyHash == key {
				foundValueHash := n.valueHash

				return &foundValueHash, nil
			}

			return nil, nil
		}
	}

	return nil, fmt.Errorf("reached end of key without finding value, erroneous node somewhere in tree")
}

func (jmt JellyfishMerkleTree) Update(oldVersion Version, newVersion Version, key Hash, valueHash Hash) error {
	currentKey := NodeKey{Version: oldVersion}
	nodeBatch := make(map[NodeKey]Node)

	keyPath, err := NewNibblePath(len(key)*2, key[:])
	if err != nil {
		return err
	}

	for _, nibble := range keyPath.Iter() {
		node, err := jmt.reader.GetNode(currentKey)
		if err != nil {
			return err
		}

		switch n := node.(type) {
		case InternalNode:
			newNodeKey := NodeKey{Version: newVersion, NibblePath: currentKey.NibblePath}
			newNode := n.Copy()
			newNode.Children[nibble] = child{version: newVersion}
			nodeBatch[newNodeKey] = newNode

			child, ok := n.Children[nibble]
			if !ok {

			}

			currentKey.Version = child.version
		case LeafNode:
		}
	}

	return nil
}
