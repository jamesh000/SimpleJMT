package jmt

import (
	"encoding/binary"
	"testing"
)

func TestNibblePathPrefixIsCanonical(t *testing.T) {
	key := hashByte(0xab)
	path, err := NewNibblePath(HashLen*2, key[:])
	if err != nil {
		t.Fatalf("NewNibblePath() error = %v", err)
	}

	empty, err := path.Prefix(0)
	if err != nil {
		t.Fatalf("Prefix(0) error = %v", err)
	}
	if *empty != (NibblePath{}) {
		t.Fatalf("Prefix(0) = %+v, want canonical empty path", empty)
	}

	for length := 0; length <= HashLen*2; length++ {
		prefix, err := path.Prefix(length)
		if err != nil {
			t.Fatalf("Prefix(%d) error = %v", length, err)
		}
		if prefix.length != length {
			t.Fatalf("Prefix(%d).length = %d", length, prefix.length)
		}

		usedBytes := (length + 1) / 2
		for i := usedBytes; i < nibbleStorageLen; i++ {
			if prefix.nibbles[i] != 0 {
				t.Fatalf("Prefix(%d) left stale byte at %d: %x", length, i, prefix.nibbles[i])
			}
		}
		if length%2 != 0 && prefix.nibbles[length/2]&0x0F != 0 {
			t.Fatalf("Prefix(%d) left garbage low nibble", length)
		}
	}
}

func TestNibblePathAppendAndIter(t *testing.T) {
	path, err := NewNibblePath(0, nil)
	if err != nil {
		t.Fatalf("NewNibblePath() error = %v", err)
	}

	want := []byte{0xa, 0x1, 0xb, 0x2, 0xc, 0x3}
	for _, nibble := range want {
		next, err := path.Append(nibble)
		if err != nil {
			t.Fatalf("Append(%x) error = %v", nibble, err)
		}
		path = &next
	}

	var got []byte
	for i, nibble := range path.Iter() {
		wantNibble, err := path.getNibble(i)
		if err != nil {
			t.Fatalf("getNibble(%d) error = %v", i, err)
		}
		if nibble != wantNibble {
			t.Fatalf("Iter()[%d] = %x, getNibble = %x", i, nibble, wantNibble)
		}
		got = append(got, nibble)
	}
	if len(got) != len(want) {
		t.Fatalf("Iter() length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Iter()[%d] = %x, want %x", i, got[i], want[i])
		}
	}

	// Iteration must stop when the consumer returns false.
	count := 0
	for range path.Iter() {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Fatalf("early-stopped Iter() count = %d, want 2", count)
	}
}

func TestNibblePathBytesRoundTrip(t *testing.T) {
	key := hashByte(0x50)
	for _, length := range []int{0, 1, 2, 3, 4, 31, 32, 63, 64} {
		path, err := NewNibblePath(length, key[:(length+1)/2])
		if err != nil {
			t.Fatalf("NewNibblePath(%d) error = %v", length, err)
		}

		encoded := path.Bytes()
		var decoded NibblePath
		got, err := decoded.FromBytes(encoded)
		if err != nil {
			t.Fatalf("FromBytes(%d) error = %v", length, err)
		}
		if *got != *path {
			t.Fatalf("FromBytes(Bytes()) = %+v, want %+v", got, path)
		}
	}
}

func TestNibblePathRejectsInvalidValues(t *testing.T) {
	if _, err := NewNibblePath(-1, nil); err == nil {
		t.Fatal("NewNibblePath(-1) succeeded")
	}
	if _, err := NewNibblePath(nibblePathMax+1, nil); err == nil {
		t.Fatal("NewNibblePath(too long) succeeded")
	}
	if _, err := NewNibblePath(1, []byte{0x1f}); err == nil {
		t.Fatal("NewNibblePath() accepted garbage low nibble")
	}
	if _, err := NewNibblePath(1, nil); err == nil {
		t.Fatal("NewNibblePath() accepted wrong data length")
	}

	fullKey := hashByte(0x01)
	full, err := NewNibblePath(nibblePathMax, fullKey[:])
	if err != nil {
		t.Fatalf("NewNibblePath(full) error = %v", err)
	}
	if _, err := full.Append(0x1); err == nil {
		t.Fatal("Append() succeeded on full path")
	}
	if _, err := full.Append(0x10); err == nil {
		t.Fatal("Append() accepted non-nibble value")
	}
	if _, err := full.Prefix(nibblePathMax + 1); err == nil {
		t.Fatal("Prefix() accepted out-of-range length")
	}
	if _, err := full.Prefix(-1); err == nil {
		t.Fatal("Prefix() accepted negative length")
	}

	var path NibblePath
	tooLong := make([]byte, 9)
	binary.BigEndian.PutUint64(tooLong, uint64(nibblePathMax+1))
	if _, err := path.FromBytes(tooLong); err == nil {
		t.Fatal("FromBytes() accepted too-long path")
	}

	invalid := NibblePath{length: 1, nibbles: [nibbleStorageLen]byte{0x1f}}
	encoded := invalid.Bytes()
	if _, err := new(NibblePath).FromBytes(encoded); err == nil {
		t.Fatal("FromBytes() accepted garbage low nibble")
	}
}
