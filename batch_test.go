package jmt

import (
	"math/rand"
	"testing"
)

func randomUpdates(t *testing.T, seed int64, count int) (map[Hash]Hash, []Hash) {
	t.Helper()

	rng := rand.New(rand.NewSource(seed))
	updates := make(map[Hash]Hash, count)
	keys := make([]Hash, 0, count)

	for len(updates) < count {
		var key, valueHash Hash
		if _, err := rng.Read(key[:]); err != nil {
			t.Fatalf("random key error = %v", err)
		}
		if _, err := rng.Read(valueHash[:]); err != nil {
			t.Fatalf("random value error = %v", err)
		}
		if _, exists := updates[key]; exists {
			continue
		}

		updates[key] = valueHash
		keys = append(keys, key)
	}

	return updates, keys
}

func TestBatchUpdateMatchesSequentialUpdate(t *testing.T) {
	const count = 200

	updates, keys := randomUpdates(t, 1, count)

	sequentialStore := NewMemStore()
	sequentialTree := NewJellyfishMerkleTree(sequentialStore, sequentialStore)
	for i, key := range keys {
		if _, err := sequentialTree.Update(Version(i), Version(i+1), key, updates[key]); err != nil {
			t.Fatalf("Update(%s) error = %v", key, err)
		}
	}

	batchStore := NewMemStore()
	batchTree := NewJellyfishMerkleTree(batchStore, batchStore)
	batchRoot, err := batchTree.BatchUpdate(0, 1, updates)
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	sequentialRoot, err := sequentialTree.Root(count)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if *batchRoot != sequentialRoot {
		t.Fatalf("BatchUpdate() root = %s, sequential root = %s", batchRoot, sequentialRoot)
	}

	for _, key := range keys {
		batchValue, err := batchTree.Lookup(1, key)
		if err != nil {
			t.Fatalf("batch Lookup(%s) error = %v", key, err)
		}
		sequentialValue, err := sequentialTree.Lookup(count, key)
		if err != nil {
			t.Fatalf("sequential Lookup(%s) error = %v", key, err)
		}
		if batchValue == nil || sequentialValue == nil || *batchValue != *sequentialValue || *batchValue != updates[key] {
			t.Fatalf("value mismatch for %s: batch=%v sequential=%v want=%s", key, batchValue, sequentialValue, updates[key])
		}
	}
}

func TestBatchUpdateExistingKeysMatchesSequentialUpdate(t *testing.T) {
	const initialCount = 80
	const updateCount = 30

	initialUpdates, keys := randomUpdates(t, 2, initialCount)
	moreUpdates, _ := randomUpdates(t, 3, updateCount)

	// Add updates to a subset of the existing keys.
	secondUpdates := make(map[Hash]Hash, updateCount)
	for i := 0; i < updateCount; i++ {
		secondUpdates[keys[i]] = moreUpdates[keys[i]]
	}

	batchStore := NewMemStore()
	batchTree := NewJellyfishMerkleTree(batchStore, batchStore)
	if _, err := batchTree.BatchUpdate(0, 1, initialUpdates); err != nil {
		t.Fatalf("initial BatchUpdate() error = %v", err)
	}
	batchRoot, err := batchTree.BatchUpdate(1, 2, secondUpdates)
	if err != nil {
		t.Fatalf("second BatchUpdate() error = %v", err)
	}

	sequentialStore := NewMemStore()
	sequentialTree := NewJellyfishMerkleTree(sequentialStore, sequentialStore)
	version := Version(0)
	for _, key := range keys {
		version++
		if _, err := sequentialTree.Update(version-1, version, key, initialUpdates[key]); err != nil {
			t.Fatalf("initial Update(%s) error = %v", key, err)
		}
	}
	for key, valueHash := range secondUpdates {
		version++
		if _, err := sequentialTree.Update(version-1, version, key, valueHash); err != nil {
			t.Fatalf("second Update(%s) error = %v", key, err)
		}
	}

	sequentialRoot, err := sequentialTree.Root(version)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if *batchRoot != sequentialRoot {
		t.Fatalf("BatchUpdate() root = %s, sequential root = %s", batchRoot, sequentialRoot)
	}
}

func TestBatchUpdateMixedExistingAndNewKeys(t *testing.T) {
	initialUpdates := map[Hash]Hash{
		hashByte(0x10): hashByte(0xa0),
		hashByte(0x20): hashByte(0xa1),
		hashByte(0x30): hashByte(0xa2),
		hashByte(0x40): hashByte(0xa3),
	}
	secondUpdates := map[Hash]Hash{
		hashByte(0x10): hashByte(0xb0), // update an existing leaf
		hashByte(0x30): hashByte(0xb1), // update another existing leaf
		hashByte(0x11): hashByte(0xc0), // split the leaf at nibble 1
		hashByte(0x31): hashByte(0xc1), // split the leaf at nibble 3
		hashByte(0x12): hashByte(0xc2), // split the already-split nibble-1 subtree
	}

	batchStore := NewMemStore()
	batchTree := NewJellyfishMerkleTree(batchStore, batchStore)
	if _, err := batchTree.BatchUpdate(0, 1, initialUpdates); err != nil {
		t.Fatalf("initial BatchUpdate() error = %v", err)
	}
	batchRoot, err := batchTree.BatchUpdate(1, 2, secondUpdates)
	if err != nil {
		t.Fatalf("second BatchUpdate() error = %v", err)
	}

	sequentialStore := NewMemStore()
	sequentialTree := NewJellyfishMerkleTree(sequentialStore, sequentialStore)
	version := Version(0)
	for _, key := range []Hash{hashByte(0x10), hashByte(0x20), hashByte(0x30), hashByte(0x40)} {
		version++
		if _, err := sequentialTree.Update(version-1, version, key, initialUpdates[key]); err != nil {
			t.Fatalf("initial Update(%s) error = %v", key, err)
		}
	}
	for _, key := range []Hash{hashByte(0x10), hashByte(0x30), hashByte(0x11), hashByte(0x31), hashByte(0x12)} {
		version++
		if _, err := sequentialTree.Update(version-1, version, key, secondUpdates[key]); err != nil {
			t.Fatalf("second Update(%s) error = %v", key, err)
		}
	}

	sequentialRoot, err := sequentialTree.Root(version)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if *batchRoot != sequentialRoot {
		t.Fatalf("BatchUpdate() root = %s, sequential root = %s", batchRoot, sequentialRoot)
	}

	want := map[Hash]Hash{
		hashByte(0x10): secondUpdates[hashByte(0x10)],
		hashByte(0x20): initialUpdates[hashByte(0x20)],
		hashByte(0x30): secondUpdates[hashByte(0x30)],
		hashByte(0x40): initialUpdates[hashByte(0x40)],
		hashByte(0x11): secondUpdates[hashByte(0x11)],
		hashByte(0x31): secondUpdates[hashByte(0x31)],
		hashByte(0x12): secondUpdates[hashByte(0x12)],
	}
	for key, wantValue := range want {
		got, err := batchTree.Lookup(2, key)
		if err != nil {
			t.Fatalf("Lookup(%s) error = %v", key, err)
		}
		if got == nil || *got != wantValue {
			t.Fatalf("Lookup(%s) = %v, want %s", key, got, wantValue)
		}
	}
}

func TestPebbleBatchUpdatePersists(t *testing.T) {
	dir := t.TempDir()

	updates, keys := randomUpdates(t, 4, 50)

	db, err := NewPebbleDB(dir)
	if err != nil {
		t.Fatalf("NewPebbleDB() error = %v", err)
	}
	tree := NewJellyfishMerkleTree(db, db)
	root, err := tree.BatchUpdate(0, 1, updates)
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
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
	storedRoot, err := tree.Root(1)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if storedRoot != *root {
		t.Fatalf("Root() = %s, want %s", storedRoot, *root)
	}

	for _, key := range keys {
		got, err := tree.Lookup(1, key)
		if err != nil {
			t.Fatalf("Lookup(%s) error = %v", key, err)
		}
		if got == nil || *got != updates[key] {
			t.Fatalf("Lookup(%s) = %v, want %s", key, got, updates[key])
		}
	}
}

func TestBatchUpdateLongCommonPrefix(t *testing.T) {
	const count = 8

	updates := make(map[Hash]Hash, count)
	keys := make([]Hash, 0, count)
	for i := 0; i < count; i++ {
		key := hashByte(0xab)
		key[HashLen-1] = byte(i)
		updates[key] = hashByte(byte(0x10 + i))
		keys = append(keys, key)
	}

	batchStore := NewMemStore()
	batchTree := NewJellyfishMerkleTree(batchStore, batchStore)
	batchRoot, err := batchTree.BatchUpdate(0, 1, updates)
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	sequentialStore := NewMemStore()
	sequentialTree := NewJellyfishMerkleTree(sequentialStore, sequentialStore)
	for i, key := range keys {
		if _, err := sequentialTree.Update(Version(i), Version(i+1), key, updates[key]); err != nil {
			t.Fatalf("Update(%s) error = %v", key, err)
		}
	}

	sequentialRoot, err := sequentialTree.Root(count)
	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if *batchRoot != sequentialRoot {
		t.Fatalf("BatchUpdate() root = %s, sequential root = %s", batchRoot, sequentialRoot)
	}

	for _, key := range keys {
		got, err := batchTree.Lookup(1, key)
		if err != nil {
			t.Fatalf("Lookup(%s) error = %v", key, err)
		}
		if got == nil || *got != updates[key] {
			t.Fatalf("Lookup(%s) = %v, want %s", key, got, updates[key])
		}
	}
}

func TestBatchUpdateEmpty(t *testing.T) {
	store := NewMemStore()
	tree := NewJellyfishMerkleTree(store, store)

	key := hashByte(0x01)
	valueHash := hashByte(0x11)

	root, err := tree.BatchUpdate(0, 1, map[Hash]Hash{key: valueHash})
	if err != nil {
		t.Fatalf("BatchUpdate() error = %v", err)
	}

	emptyRoot, err := tree.BatchUpdate(1, 2, nil)
	if err != nil {
		t.Fatalf("empty BatchUpdate() error = %v", err)
	}
	if *emptyRoot != *root {
		t.Fatalf("empty BatchUpdate() root = %s, want %s", emptyRoot, root)
	}

	storedRoot, err := tree.Root(2)
	if err != nil {
		t.Fatalf("Root(2) error = %v", err)
	}
	if storedRoot != *root {
		t.Fatalf("Root(2) = %s, want %s", storedRoot, *root)
	}

	got, err := tree.Lookup(2, key)
	if err != nil {
		t.Fatalf("Lookup(2) error = %v", err)
	}
	if got == nil || *got != valueHash {
		t.Fatalf("Lookup(2) = %v, want %s", got, valueHash)
	}

	emptyStore := NewMemStore()
	emptyTree := NewJellyfishMerkleTree(emptyStore, emptyStore)
	emptyTreeRoot, err := emptyTree.BatchUpdate(0, 1, nil)
	if err != nil {
		t.Fatalf("empty-tree BatchUpdate() error = %v", err)
	}
	if *emptyTreeRoot != EmptyRootHash {
		t.Fatalf("empty-tree BatchUpdate() root = %s, want %s", emptyTreeRoot, EmptyRootHash)
	}
}
