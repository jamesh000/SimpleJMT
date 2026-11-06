package jmt

import (
	"math/rand"
	"testing"
)

func TestProofsInclusionAndNonInclusion(t *testing.T) {
	updates, keys := randomUpdates(t, 10, 100)

	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)
	root, err := tree.BatchUpdate(0, 1, updates)
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	for _, key := range keys {
		wantValueHash := updates[key]
		valueHash, valueIndex, proof, err := tree.GetWithProof(1, key)
		if err != nil {
			t.Fatalf("GetWithProof(%s) error = %v", key, err)
		}
		if valueHash == nil || *valueHash != wantValueHash {
			t.Fatalf("GetWithProof(%s) value hash = %v, want %s", key, valueHash, wantValueHash)
		}
		if valueIndex == nil || valueIndex.KeyHash != key || valueIndex.Version != 1 {
			t.Fatalf("GetWithProof(%s) value index = %v, want key %s version 1", key, valueIndex, key)
		}
		if proof.Leaf == nil || proof.Leaf.KeyHash != key || proof.Leaf.ValueHash != wantValueHash {
			t.Fatalf("GetWithProof(%s) leaf = %+v, want key %s value %s", key, proof.Leaf, key, wantValueHash)
		}
		if err := proof.Verify(*root, key, &wantValueHash); err != nil {
			t.Fatalf("Verify(inclusion, %s) error = %v", key, err)
		}

		wrongValueHash := hashByte(0xff)
		if err := proof.Verify(*root, key, &wrongValueHash); err == nil {
			t.Fatalf("Verify(inclusion, %s) succeeded with wrong value hash", key)
		}
		if err := proof.Verify(*root, key, nil); err == nil {
			t.Fatalf("Verify(non-inclusion, %s) succeeded for an existing key", key)
		}
	}

	rng := rand.New(rand.NewSource(11))
	checkedMissing := 0
	for checkedMissing < 50 {
		var missingKey Hash
		if _, err := rng.Read(missingKey[:]); err != nil {
			t.Fatalf("random key error = %v", err)
		}
		if _, exists := updates[missingKey]; exists {
			continue
		}
		checkedMissing++

		valueHash, valueIndex, proof, err := tree.GetWithProof(1, missingKey)
		if err != nil {
			t.Fatalf("GetWithProof(missing %s) error = %v", missingKey, err)
		}
		if valueHash != nil || valueIndex != nil {
			t.Fatalf("GetWithProof(missing %s) = (%v, %v), want nil", missingKey, valueHash, valueIndex)
		}
		if proof == nil {
			t.Fatalf("GetWithProof(missing %s) returned nil proof", missingKey)
		}
		if err := proof.Verify(*root, missingKey, nil); err != nil {
			t.Fatalf("Verify(non-inclusion, %s) error = %v", missingKey, err)
		}

		randomValueHash := hashByte(0xab)
		if err := proof.Verify(*root, missingKey, &randomValueHash); err == nil {
			t.Fatalf("Verify(inclusion, %s) succeeded for a missing key", missingKey)
		}
	}
}

func TestProofLongCommonPrefix(t *testing.T) {
	const count = 8

	updates := make(map[Hash]Hash, count)
	keys := make([]Hash, 0, count)
	for i := 0; i < count; i++ {
		key := hashByte(0xab)
		key[HashLen-1] = byte(i)
		updates[key] = hashByte(byte(0x10 + i))
		keys = append(keys, key)
	}

	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)
	root, err := tree.BatchUpdate(0, 1, updates)
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	for _, key := range keys {
		wantValueHash := updates[key]
		_, _, proof, err := tree.GetWithProof(1, key)
		if err != nil {
			t.Fatalf("GetWithProof(%s) error = %v", key, err)
		}
		if len(proof.Siblings) != HashLen*8 {
			t.Fatalf("GetWithProof(%s) sibling count = %d, want %d", key, len(proof.Siblings), HashLen*8)
		}
		if err := proof.Verify(*root, key, &wantValueHash); err != nil {
			t.Fatalf("Verify(inclusion, %s) error = %v", key, err)
		}
	}

	missingKey := hashByte(0xab)
	missingKey[HashLen-1] = 0x08
	valueHash, valueIndex, proof, err := tree.GetWithProof(1, missingKey)
	if err != nil {
		t.Fatalf("GetWithProof(missing) error = %v", err)
	}
	if valueHash != nil || valueIndex != nil {
		t.Fatalf("GetWithProof(missing) = (%v, %v), want nil", valueHash, valueIndex)
	}
	if err := proof.Verify(*root, missingKey, nil); err != nil {
		t.Fatalf("Verify(non-inclusion, missing) error = %v", err)
	}
}

func TestProofEmptyTree(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)
	key := hashByte(0x01)

	valueHash, valueIndex, proof, err := tree.GetWithProof(0, key)
	if err != nil {
		t.Fatalf("GetWithProof() error = %v", err)
	}
	if valueHash != nil || valueIndex != nil {
		t.Fatalf("GetWithProof() = (%v, %v), want nil", valueHash, valueIndex)
	}
	if proof.Leaf != nil || len(proof.Siblings) != 0 {
		t.Fatalf("GetWithProof() proof = %+v, want empty proof", proof)
	}
	if err := proof.Verify(EmptyRootHash, key, nil); err != nil {
		t.Fatalf("Verify(non-inclusion, empty tree) error = %v", err)
	}

	someValueHash := hashByte(0x11)
	if err := proof.Verify(EmptyRootHash, key, &someValueHash); err == nil {
		t.Fatalf("Verify(inclusion, empty tree) succeeded")
	}
}

func TestProofTerminalLeafNonInclusion(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key1 := hashByte(0x10)
	key2 := hashByte(0x20)
	root, err := tree.BatchUpdate(0, 1, map[Hash]Hash{
		key1: hashByte(0xa0),
		key2: hashByte(0xa1),
	})
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	missingKey := hashByte(0x11)
	valueHash, valueIndex, proof, err := tree.GetWithProof(1, missingKey)
	if err != nil {
		t.Fatalf("GetWithProof(missing) error = %v", err)
	}
	if valueHash != nil || valueIndex != nil {
		t.Fatalf("GetWithProof(missing) = (%v, %v), want nil", valueHash, valueIndex)
	}
	if proof.Leaf == nil || proof.Leaf.KeyHash != key1 {
		t.Fatalf("GetWithProof(missing) leaf = %+v, want terminal leaf %s", proof.Leaf, key1)
	}
	if err := proof.Verify(*root, missingKey, nil); err != nil {
		t.Fatalf("Verify(non-inclusion) error = %v", err)
	}
	if err := proof.Verify(*root, proof.Leaf.KeyHash, nil); err == nil {
		t.Fatal("Verify(non-inclusion) succeeded when the key exists in the proof")
	}
}

func TestProofTampering(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)
	root, err := tree.BatchUpdate(0, 1, map[Hash]Hash{
		key:            valueHash,
		hashByte(0x02): hashByte(0x22),
		hashByte(0x03): hashByte(0x33),
	})
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	_, _, proof, err := tree.GetWithProof(1, key)
	if err != nil {
		t.Fatalf("GetWithProof() error = %v", err)
	}
	if len(proof.Siblings) == 0 {
		t.Fatal("test proof unexpectedly has no siblings")
	}

	tamperedLeaf := &SparseMerkleProof{
		Leaf: &SparseMerkleLeafNode{
			KeyHash:   proof.Leaf.KeyHash,
			ValueHash: hashByte(0x44),
		},
		Siblings: append([]Hash(nil), proof.Siblings...),
	}
	if err := tamperedLeaf.Verify(*root, key, &tamperedLeaf.Leaf.ValueHash); err == nil {
		t.Fatal("Verify() accepted a tampered leaf")
	}

	tamperedSibling := &SparseMerkleProof{
		Leaf:     proof.Leaf,
		Siblings: append([]Hash(nil), proof.Siblings...),
	}
	tamperedSibling.Siblings[0] = hashByte(0x55)
	if err := tamperedSibling.Verify(*root, key, &valueHash); err == nil {
		t.Fatal("Verify() accepted a tampered sibling")
	}

	truncated := &SparseMerkleProof{
		Leaf:     proof.Leaf,
		Siblings: append([]Hash(nil), proof.Siblings[:len(proof.Siblings)-1]...),
	}
	if err := truncated.Verify(*root, key, &valueHash); err == nil {
		t.Fatal("Verify() accepted a truncated proof")
	}
}

func TestProofSingleLeafRoot(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)
	root, err := tree.Update(0, 1, key, valueHash)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	_, _, proof, err := tree.GetWithProof(1, key)
	if err != nil {
		t.Fatalf("GetWithProof() error = %v", err)
	}
	if len(proof.Siblings) != 0 {
		t.Fatalf("single-leaf proof sibling count = %d, want 0", len(proof.Siblings))
	}
	if err := proof.Verify(*root, key, &valueHash); err != nil {
		t.Fatalf("Verify(inclusion) error = %v", err)
	}

	missingKey := hashByte(0x02)
	_, _, missingProof, err := tree.GetWithProof(1, missingKey)
	if err != nil {
		t.Fatalf("GetWithProof(missing) error = %v", err)
	}
	if len(missingProof.Siblings) != 0 || missingProof.Leaf == nil {
		t.Fatalf("single-leaf non-inclusion proof = %+v", missingProof)
	}
	if err := missingProof.Verify(*root, missingKey, nil); err != nil {
		t.Fatalf("Verify(non-inclusion) error = %v", err)
	}
}

func TestProofCollapsedInternalRoot(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)
	leaf := NewLeafNode(key, valueHash, ValueIndex{KeyHash: key, Version: 1})

	leafPath, err := NewNibblePath(1, []byte{0x00})
	if err != nil {
		t.Fatalf("NewNibblePath() error = %v", err)
	}
	leafKey := NodeKey{Version: 1, NibblePath: *leafPath}

	root := NewInternalNode()
	root.insertChild(0x0, 1, leaf.Hash(), leafType)
	rootKey := NodeKey{Version: 1}

	if err := store.WriteNodeBatch(map[NodeKey]Node{
		rootKey: root,
		leafKey: leaf,
	}); err != nil {
		t.Fatalf("WriteNodeBatch() error = %v", err)
	}

	rootHash := root.Hash()
	if rootHash != leaf.Hash() {
		t.Fatalf("collapsed internal root hash = %s, want leaf hash %s", rootHash, leaf.Hash())
	}

	gotValueHash, gotValueIndex, proof, err := tree.GetWithProof(1, key)
	if err != nil {
		t.Fatalf("GetWithProof() error = %v", err)
	}
	if gotValueHash == nil || *gotValueHash != valueHash {
		t.Fatalf("GetWithProof() value hash = %v, want %s", gotValueHash, valueHash)
	}
	if gotValueIndex == nil || *gotValueIndex != leaf.valueIndex {
		t.Fatalf("GetWithProof() value index = %v, want %+v", gotValueIndex, leaf.valueIndex)
	}
	if len(proof.Siblings) != 0 {
		t.Fatalf("collapsed internal proof sibling count = %d, want 0", len(proof.Siblings))
	}
	if err := proof.Verify(rootHash, key, &valueHash); err != nil {
		t.Fatalf("Verify(inclusion) error = %v", err)
	}
}

func TestProofAfterEmptyBatch(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)
	if _, err := tree.Update(0, 1, key, valueHash); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	root, err := tree.BatchUpdate(1, 2, nil)
	if err != nil {
		t.Fatalf("empty BatchUpdate() error = %v", err)
	}

	_, _, proof, err := tree.GetWithProof(2, key)
	if err != nil {
		t.Fatalf("GetWithProof() error = %v", err)
	}
	if err := proof.Verify(*root, key, &valueHash); err != nil {
		t.Fatalf("Verify(inclusion after empty batch) error = %v", err)
	}
}

func TestProofHistoricalVersion(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key1 := hashByte(0x01)
	key2 := hashByte(0x02)
	value1 := hashByte(0x11)
	value2 := hashByte(0x22)

	root1, err := tree.Update(0, 1, key1, value1)
	if err != nil {
		t.Fatalf("Update(key1) error = %v", err)
	}
	if _, err := tree.Update(1, 2, key2, value2); err != nil {
		t.Fatalf("Update(key2) error = %v", err)
	}

	_, _, proof1, err := tree.GetWithProof(1, key1)
	if err != nil {
		t.Fatalf("GetWithProof(key1 at v1) error = %v", err)
	}
	if err := proof1.Verify(*root1, key1, &value1); err != nil {
		t.Fatalf("Verify(key1 at v1) error = %v", err)
	}

	_, _, proof2, err := tree.GetWithProof(1, key2)
	if err != nil {
		t.Fatalf("GetWithProof(key2 at v1) error = %v", err)
	}
	if err := proof2.Verify(*root1, key2, nil); err != nil {
		t.Fatalf("Verify(key2 non-inclusion at v1) error = %v", err)
	}
}
