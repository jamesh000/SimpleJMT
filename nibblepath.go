package jmt

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"iter"
)

const nibblePathMax = 2 * HashLen
const nibbleStorageLen = (nibblePathMax + 1) / 2

type NibblePath struct {
	length  int
	nibbles [nibbleStorageLen]byte
}

// normalize clears storage that is not part of the logical path. NibblePath is
// used as part of NodeKey, which is a map key, so equal paths must have
// byte-for-byte equal storage.
func (path *NibblePath) normalize() {
	usedBytes := (path.length + 1) / 2
	for i := usedBytes; i < nibbleStorageLen; i++ {
		path.nibbles[i] = 0
	}

	// If the path ends on a high nibble, clear the unused low nibble of the
	// final byte so the path remains well-formed.
	if path.length%2 != 0 {
		path.nibbles[path.length/2] &= 0xF0
	}
}

func NewNibblePath(length int, data []byte) (*NibblePath, error) {
	if length > nibblePathMax || length < 0 {
		return nil, fmt.Errorf("erroneous length")
	}

	if len(data) != (length+1)/2 {
		return nil, fmt.Errorf("length does not match provided data")
	}

	if length%2 != 0 && (data[len(data)-1]&0xF) != 0 {
		return nil, fmt.Errorf("garbage bits at end of data")
	}

	path := &NibblePath{
		length:  length,
		nibbles: [nibbleStorageLen]byte{},
	}
	copy(path.nibbles[:], data)

	return path, nil
}

func (path *NibblePath) getNibble(pos int) (byte, error) {
	if pos >= path.length || pos < 0 {
		return 0, fmt.Errorf("out of range nibble access")
	}

	b := path.nibbles[pos/2]
	shift := ((pos + 1) % 2) * 4
	return (b >> shift) & 0x0F, nil
}

func (path *NibblePath) Iter() iter.Seq2[int, byte] {
	return func(yield func(int, byte) bool) {
		for i := 0; i < int(path.length); i++ {
			b := path.nibbles[i/2]
			shift := ((i + 1) % 2) * 4 // get the first half of the byte on an even number
			nibble := (b >> shift) & 0x0F
			if !yield(i, nibble) {
				return
			}
		}
	}
}

func (fromPath *NibblePath) Append(nibble byte) (NibblePath, error) {
	path := *fromPath
	if path.length+1 > nibblePathMax {
		return NibblePath{}, fmt.Errorf("Tried to append to full nibble path")
	}

	if (nibble & 0xF0) != 0 {
		return NibblePath{}, fmt.Errorf("Invalid nibble format for appending")
	}

	if path.length%2 == 0 {
		path.nibbles[path.length/2] = nibble << 4
	} else {
		path.nibbles[path.length/2] |= nibble
	}

	path.length++
	path.normalize()

	return path, nil
}

// Prefix returns a new NibblePath containing the first length nibbles of path.
func (path *NibblePath) Prefix(length int) (*NibblePath, error) {
	if length < 0 || length > path.length {
		return nil, fmt.Errorf("prefix length out of range")
	}

	prefix := &NibblePath{
		length: length,
	}
	copy(prefix.nibbles[:], path.nibbles[:])
	prefix.normalize()

	return prefix, nil
}

func (path *NibblePath) Bytes() []byte {
	usedBytes := (path.length + 1) / 2
	encoded := make([]byte, 8+usedBytes)
	binary.BigEndian.PutUint64(encoded[:8], uint64(path.length))
	copy(encoded[8:], path.nibbles[:usedBytes])
	return encoded
}

func (path *NibblePath) FromBytes(data []byte) (*NibblePath, error) {
	const lengthPrefixLen = 8
	if len(data) < lengthPrefixLen {
		return nil, fmt.Errorf("nibblepath encoding is too short")
	}

	length := binary.BigEndian.Uint64(data[:lengthPrefixLen])
	if length > nibblePathMax {
		return nil, fmt.Errorf("erroneous length")
	}

	pathLen := int(length)
	usedBytes := (pathLen + 1) / 2
	if len(data) != lengthPrefixLen+usedBytes {
		return nil, fmt.Errorf("nibblepath length does not match actual size")
	}

	path.nibbles = [nibbleStorageLen]byte{}
	copy(path.nibbles[:], data[lengthPrefixLen:])

	if pathLen%2 != 0 && path.nibbles[pathLen/2]&0xF != 0 {
		return nil, fmt.Errorf("garbage bits at end of bytes")
	}

	path.length = pathLen
	path.normalize()

	return path, nil
}

func (path NibblePath) String() string {
	s := hex.EncodeToString(path.nibbles[:])
	return s[:path.length]
}
