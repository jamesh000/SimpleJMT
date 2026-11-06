package jmt

import (
	"bytes"
	"testing"
)

func TestValueKeyEncoding(t *testing.T) {
	key := ValueKey{
		KeyHash: hashByte(0x42),
		Version: 1234,
	}

	encoded := key.Encode()
	if len(encoded) != ValueKeyLen {
		t.Fatalf("Encode() length = %d, want %d", len(encoded), ValueKeyLen)
	}

	decoded, err := DecodeValueKey(encoded)
	if err != nil {
		t.Fatalf("DecodeValueKey() error = %v", err)
	}
	if decoded != key {
		t.Fatalf("DecodeValueKey() = %+v, want %+v", decoded, key)
	}
}

func TestLeafValueIndexSerialization(t *testing.T) {
	keyHash := hashByte(0x01)
	valueHash := hashByte(0x11)
	valueIndex := ValueIndex{KeyHash: keyHash, Version: 7}
	leaf := NewLeafNode(keyHash, valueHash, valueIndex)

	data, err := leaf.SerializeNode()
	if err != nil {
		t.Fatalf("SerializeNode() error = %v", err)
	}

	node, err := DeserializeNode(data)
	if err != nil {
		t.Fatalf("DeserializeNode() error = %v", err)
	}

	decoded, ok := node.(*LeafNode)
	if !ok {
		t.Fatalf("DeserializeNode() returned %T, want *LeafNode", node)
	}
	if decoded.keyHash != keyHash {
		t.Fatalf("key hash = %s, want %s", decoded.keyHash, keyHash)
	}
	if decoded.valueHash != valueHash {
		t.Fatalf("value hash = %s, want %s", decoded.valueHash, valueHash)
	}
	if decoded.valueIndex != valueIndex {
		t.Fatalf("value index = %+v, want %+v", decoded.valueIndex, valueIndex)
	}
	if decoded.Hash() != leaf.Hash() {
		t.Fatalf("deserialized leaf hash = %s, want %s", decoded.Hash(), leaf.Hash())
	}
}

func TestValueIndexAcrossSplitsAndUpdates(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key1 := hashByte(0x01)
	key2 := hashByte(0x02)

	if _, err := tree.Update(0, 1, key1, hashByte(0x11)); err != nil {
		t.Fatalf("insert key1 error = %v", err)
	}
	if _, err := tree.Update(1, 2, key2, hashByte(0x22)); err != nil {
		t.Fatalf("insert key2 error = %v", err)
	}

	checkIndex := func(version Version, key Hash, want Version) {
		t.Helper()
		_, index, err := tree.LookupWithIndex(version, key)
		if err != nil {
			t.Fatalf("LookupWithIndex(%d, %s) error = %v", version, key, err)
		}
		if index == nil || index.KeyHash != key || index.Version != want {
			t.Fatalf("LookupWithIndex(%d, %s) = %v, want key %s version %d", version, key, index, key, want)
		}
	}

	// Splitting the tree to insert key2 must preserve key1's old value index.
	checkIndex(2, key1, 1)
	checkIndex(2, key2, 2)

	if _, err := tree.Update(2, 3, key1, hashByte(0x33)); err != nil {
		t.Fatalf("update key1 error = %v", err)
	}
	checkIndex(3, key1, 3)
	checkIndex(3, key2, 2)
}

func TestValueKeyVersionOrdering(t *testing.T) {
	keyHash := hashByte(0x01)
	version1 := ValueKey{KeyHash: keyHash, Version: 1}.Encode()
	version2 := ValueKey{KeyHash: keyHash, Version: 2}.Encode()
	version3 := ValueKey{KeyHash: keyHash, Version: 3}.Encode()

	// Newer versions must sort before older versions.
	if bytes.Compare(version3, version2) >= 0 {
		t.Fatal("version 3 does not sort before version 2")
	}
	if bytes.Compare(version2, version1) >= 0 {
		t.Fatal("version 2 does not sort before version 1")
	}
}

func TestMemStoreGetValueVersions(t *testing.T) {
	store := NewMemStore()
	key := hashByte(0x01)

	writes := []struct {
		version Version
		value   Value
	}{
		{version: 1, value: Value("one")},
		{version: 3, value: Value("three")},
		{version: 5, value: nil},
	}
	for _, write := range writes {
		if err := store.WriteValue(ValueKey{KeyHash: key, Version: write.version}, write.value); err != nil {
			t.Fatalf("WriteValue(%d) error = %v", write.version, err)
		}
	}

	checks := []struct {
		version Version
		want    Value
	}{
		{version: 1, want: Value("one")},
		{version: 2, want: Value("one")},
		{version: 3, want: Value("three")},
		{version: 4, want: Value("three")},
		{version: 5, want: nil},
		{version: 6, want: nil},
	}
	for _, check := range checks {
		got, err := store.GetValue(key, check.version)
		if err != nil {
			t.Fatalf("GetValue(%d) error = %v", check.version, err)
		}
		if !bytes.Equal(got, check.want) {
			t.Fatalf("GetValue(%d) = %q, want %q", check.version, got, check.want)
		}
	}

	if _, err := store.GetValue(key, 0); err == nil {
		t.Fatal("GetValue() succeeded before the first value")
	}
	if _, err := store.GetValue(hashByte(0x02), 100); err == nil {
		t.Fatal("GetValue() succeeded for a missing key")
	}
}

func TestUpdateStoresValueIndex(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)

	if _, err := tree.Update(0, 1, key, valueHash); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	gotValueHash, gotIndex, err := tree.LookupWithIndex(1, key)
	if err != nil {
		t.Fatalf("LookupWithIndex() error = %v", err)
	}
	if gotValueHash == nil || *gotValueHash != valueHash {
		t.Fatalf("LookupWithIndex() value hash = %v, want %s", gotValueHash, valueHash)
	}

	wantIndex := ValueIndex{KeyHash: key, Version: 1}
	if gotIndex == nil || *gotIndex != wantIndex {
		t.Fatalf("LookupWithIndex() index = %v, want %+v", gotIndex, wantIndex)
	}

	if err := store.WriteValue(*gotIndex, Value("hello")); err != nil {
		t.Fatalf("WriteValue() error = %v", err)
	}
	gotValue, err := store.GetValue(key, 1)
	if err != nil {
		t.Fatalf("GetValue() error = %v", err)
	}
	if !bytes.Equal(gotValue, []byte("hello")) {
		t.Fatalf("GetValue() = %q, want %q", gotValue, "hello")
	}

	tombstoneKey := ValueKey{KeyHash: key, Version: 2}
	if err := store.WriteValue(tombstoneKey, nil); err != nil {
		t.Fatalf("WriteValue(tombstone) error = %v", err)
	}
	gotValue, err = store.GetValue(key, 2)
	if err != nil {
		t.Fatalf("GetValue() after tombstone error = %v", err)
	}
	if gotValue != nil {
		t.Fatalf("GetValue() after tombstone = %q, want nil", gotValue)
	}
}
