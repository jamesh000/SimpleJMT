package main

import "fmt"

type Version = uint64

type JellyfishMerkleTree struct {
	reader TreeReader
	roots  map[Version]NodeKey
}

func (jmt JellyfishMerkleTree) lookup(version Version, key KeyHash) (*Value, error) {
	root, ok := jmt.roots[version]
	if !ok {
		return nil, fmt.Errorf("No root node for version %v", version)
	}

	currentKey := root
	nibblePath, err := NewNibblePath(len(key), key)
	if err != nil {
		return nil, err
	}

	for _, nibble := range nibblePath.Iter() {
		node, err := jmt.reader.GetNode(currentKey)
		if err != nil {
			return nil, err
		}

		switch n := node.(type) {
		case InternalNode:
			// pull the current nibble
			if n.Bitmap&(1<<nibble) == 0 {
				return nil, nil
			}
		}
	}
}
