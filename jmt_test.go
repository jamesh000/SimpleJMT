package jmt

import (
	"encoding/base64"
	"testing"
)

// hashByte returns a Hash with every byte set to b, giving simple deterministic
// test keys and value hashes.
func hashByte(b byte) Hash {
	var h Hash
	for i := range h {
		h[i] = b
	}
	return h
}

// mustHash decodes a base64-encoded golden hash. It mirrors Hash.String so the
// golden values can be read as plain strings.
func mustHash(t *testing.T, s string) Hash {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("invalid base64 golden hash %q: %v", s, err)
	}
	if len(decoded) != HashLen {
		t.Fatalf("golden hash %q has length %d, want %d", s, len(decoded), HashLen)
	}

	var h Hash
	copy(h[:], decoded)
	return h
}

func TestGoldenRootHashes(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key1 := hashByte(0x01)
	value1 := hashByte(0x11)
	key2 := hashByte(0x02)
	value2 := hashByte(0x22)
	key3 := hashByte(0x03)
	value3 := hashByte(0x33)
	value1Updated := hashByte(0x44)

	tests := []struct {
		name       string
		oldVersion Version
		newVersion Version
		key        Hash
		valueHash  Hash
		wantRoot   Hash
	}{
		{
			name:       "insert key1",
			oldVersion: 0,
			newVersion: 1,
			key:        key1,
			valueHash:  value1,
			wantRoot:   mustHash(t, "n3k5scgZamVEkBs2am56u3rWUeAtPTMXhEPg6rwE8Dc="),
		},
		{
			name:       "insert key2",
			oldVersion: 1,
			newVersion: 2,
			key:        key2,
			valueHash:  value2,
			wantRoot:   mustHash(t, "eFG9PRzCu/xEm5Y5dSoxC3+mwWoRtnCwaU3xlKxiEXE="),
		},
		{
			name:       "update key1",
			oldVersion: 2,
			newVersion: 3,
			key:        key1,
			valueHash:  value1Updated,
			wantRoot:   mustHash(t, "fPQKbWdQZAg7F4Ty1ci0KegjqzETZ1XO/Z4+xtUILXM="),
		},
		{
			name:       "insert key3",
			oldVersion: 3,
			newVersion: 4,
			key:        key3,
			valueHash:  value3,
			wantRoot:   mustHash(t, "mfFwhhhq5VR4+6wcA80YdF8pasMqg8b0clw5psgQsv8="),
		},
	}

	// The subtests share the same tree and must run in order, so they must not
	// be marked t.Parallel.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRoot, err := tree.Update(tt.oldVersion, tt.newVersion, tt.key, tt.valueHash)
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
			if *gotRoot != tt.wantRoot {
				t.Fatalf("Update() root hash = %s, want %s", gotRoot, tt.wantRoot)
			}
		})
	}

	wantValues := map[Hash]Hash{
		key1: value1Updated,
		key2: value2,
		key3: value3,
	}
	for key, want := range wantValues {
		got, err := tree.Lookup(4, key)
		if err != nil {
			t.Fatalf("Lookup(%s) error = %v", key, err)
		}
		if got == nil || *got != want {
			t.Fatalf("Lookup(%s) = %v, want %s", key, got, want)
		}
	}
}
