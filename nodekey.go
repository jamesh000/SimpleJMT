package jmt

import (
	"encoding/binary"
	"fmt"
)

const nodeKeyHeaderLen = 8 + 1

// Encode serializes a NodeKey using the same layout as Aptos:
//
//	version (big endian) || number of nibbles || nibble path bytes
func (key NodeKey) Encode() ([]byte, error) {
	pathLen := key.NibblePath.length
	if pathLen < 0 || pathLen > nibblePathMax {
		return nil, fmt.Errorf("invalid nibble path length %d", pathLen)
	}

	pathBytes := (pathLen + 1) / 2
	encoded := make([]byte, nodeKeyHeaderLen+pathBytes)
	binary.BigEndian.PutUint64(encoded[:8], key.Version)
	encoded[8] = byte(pathLen)
	copy(encoded[nodeKeyHeaderLen:], key.NibblePath.nibbles[:pathBytes])

	return encoded, nil
}

func DecodeNodeKey(data []byte) (NodeKey, error) {
	if len(data) < nodeKeyHeaderLen {
		return NodeKey{}, fmt.Errorf("node key has length %d, want at least %d", len(data), nodeKeyHeaderLen)
	}

	version := binary.BigEndian.Uint64(data[:8])
	pathLen := int(data[8])
	if pathLen > nibblePathMax {
		return NodeKey{}, fmt.Errorf("invalid nibble path length %d", pathLen)
	}

	pathBytes := (pathLen + 1) / 2
	if len(data) != nodeKeyHeaderLen+pathBytes {
		return NodeKey{}, fmt.Errorf("node key has length %d, want %d", len(data), nodeKeyHeaderLen+pathBytes)
	}

	path, err := NewNibblePath(pathLen, data[nodeKeyHeaderLen:])
	if err != nil {
		return NodeKey{}, err
	}

	return NodeKey{
		Version:    version,
		NibblePath: *path,
	}, nil
}
