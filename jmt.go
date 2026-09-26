package jmt

import "fmt"

type Version = uint64

type JellyfishMerkleTree struct {
	reader TreeReader
}

func (jmt JellyfishMerkleTree) Lookup(version Version, key KeyHash) (*Hash, error) {
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
