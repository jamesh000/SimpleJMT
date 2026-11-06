package jmt

import (
	"encoding/binary"
	"fmt"
)

const ValueKeyLen = HashLen + 8

// ValueKey is the key of the versioned value store. Values are kept outside of
// the JMT and are indexed by the hashed state key and the version at which the
// value was written. A nil Value is a tombstone.
type ValueKey struct {
	KeyHash Hash
	Version Version
}

// ValueIndex is the pointer stored in a leaf that identifies its value entry.
// It uses the same key layout as the value schema.
type ValueIndex = ValueKey

// Encode serializes a value key in the same layout as Aptos:
//
//	key hash || bitwise-inverted version (big endian)
//
// Inverting the version makes newer versions sort before older versions, so a
// forward seek at a target version finds the newest entry at or before it.
func (key ValueKey) Encode() []byte {
	encoded := make([]byte, ValueKeyLen)
	copy(encoded[:HashLen], key.KeyHash[:])
	binary.BigEndian.PutUint64(encoded[HashLen:], ^key.Version)
	return encoded
}

func DecodeValueKey(data []byte) (ValueKey, error) {
	if len(data) != ValueKeyLen {
		return ValueKey{}, fmt.Errorf("value key has length %d, want %d", len(data), ValueKeyLen)
	}

	var key ValueKey
	copy(key.KeyHash[:], data[:HashLen])
	key.Version = ^binary.BigEndian.Uint64(data[HashLen:])
	return key, nil
}
