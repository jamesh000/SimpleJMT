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
