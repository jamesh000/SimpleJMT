package jmt

import (
	"errors"
	"testing"
)

func TestNodeKeyEncodingRoundTrip(t *testing.T) {
	fullKey := hashByte(0x42)
	paths := []*NibblePath{
		mustNibblePath(t, 0, nil),
		mustNibblePath(t, 1, []byte{0x10}),
		mustNibblePath(t, 2, []byte{0xab}),
		mustNibblePath(t, 3, []byte{0xab, 0xc0}),
		mustNibblePath(t, nibblePathMax, fullKey[:]),
	}

	for _, path := range paths {
		key := NodeKey{Version: 42, NibblePath: *path}
		encoded, err := key.Encode()
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		decoded, err := DecodeNodeKey(encoded)
		if err != nil {
			t.Fatalf("DecodeNodeKey() error = %v", err)
		}
		if decoded != key {
			t.Fatalf("DecodeNodeKey() = %+v, want %+v", decoded, key)
		}
	}
}

func TestPebbleDBPersistsTree(t *testing.T) {
	dir := t.TempDir()

	db, err := NewPebbleDB(dir)
	if err != nil {
		t.Fatalf("NewPebbleDB() error = %v", err)
	}

	tree := NewJellyfishMerkleTree(db, db)
	key1 := hashByte(0x01)
	key2 := hashByte(0x02)
	value1 := hashByte(0x11)
	value2 := hashByte(0x22)

	if _, err := tree.Update(0, 1, key1, value1); err != nil {
		t.Fatalf("Update(key1) error = %v", err)
	}
	root, err := tree.Update(1, 2, key2, value2)
	if err != nil {
		t.Fatalf("Update(key2) error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	db, err = NewPebbleDB(dir)
	if err != nil {
		t.Fatalf("reopen NewPebbleDB() error = %v", err)
	}
	defer db.Close()

	tree = NewJellyfishMerkleTree(db, db)

	gotRoot, err := tree.Root(2)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if gotRoot != *root {
		t.Fatalf("Root() = %s, want %s", gotRoot, *root)
	}

	wantValues := map[Hash]Hash{
		key1: value1,
		key2: value2,
	}
	for key, want := range wantValues {
		got, err := tree.Lookup(2, key)
		if err != nil {
			t.Fatalf("Lookup(%s) error = %v", key, err)
		}
		if got == nil || *got != want {
			t.Fatalf("Lookup(%s) = %v, want %s", key, got, want)
		}
	}

	if _, err := db.GetNode(NodeKey{Version: 99}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("GetNode(missing) error = %v, want ErrNodeNotFound", err)
	}
}

func TestPebbleDBWriteNodeBatch(t *testing.T) {
	db, err := NewPebbleDB(t.TempDir())
	if err != nil {
		t.Fatalf("NewPebbleDB() error = %v", err)
	}
	defer db.Close()

	path := mustNibblePath(t, 2, []byte{0xab})
	key := NodeKey{Version: 7, NibblePath: *path}
	keyHash := hashByte(0x01)
	valueHash := hashByte(0x11)
	valueIndex := ValueIndex{KeyHash: keyHash, Version: 7}
	leaf := NewLeafNode(keyHash, valueHash, valueIndex)

	if err := db.WriteNodeBatch(map[NodeKey]Node{key: leaf}); err != nil {
		t.Fatalf("WriteNodeBatch() error = %v", err)
	}

	node, err := db.GetNode(key)
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}

	gotLeaf, ok := node.(*LeafNode)
	if !ok {
		t.Fatalf("GetNode() returned %T, want *LeafNode", node)
	}
	if gotLeaf.Hash() != leaf.Hash() {
		t.Fatalf("GetNode() hash = %s, want %s", gotLeaf.Hash(), leaf.Hash())
	}
	if gotLeaf.valueIndex != valueIndex {
		t.Fatalf("GetNode() value index = %+v, want %+v", gotLeaf.valueIndex, valueIndex)
	}
}

func mustNibblePath(t *testing.T, length int, data []byte) *NibblePath {
	t.Helper()

	path, err := NewNibblePath(length, data)
	if err != nil {
		t.Fatalf("NewNibblePath(%d, %x) error = %v", length, data, err)
	}
	return path
}

func TestDecodeNodeKeyRejectsMalformed(t *testing.T) {
	root, err := (NodeKey{Version: 1}).Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	cases := [][]byte{
		nil,
		root[:8],
		append(append([]byte(nil), root...), 0x00),
	}
	tooLongPath := append([]byte(nil), root...)
	tooLongPath[8] = nibblePathMax + 1
	cases = append(cases, tooLongPath)

	for _, data := range cases {
		if _, err := DecodeNodeKey(data); err == nil {
			t.Fatalf("DecodeNodeKey(%x) succeeded, want error", data)
		}
	}
}
