package main

import (
	"fmt"
	"iter"
)

type NibblePath struct {
	length  int
	nibbles [32]byte
}

func NewNibblePath(length int, data []byte) (NibblePath, error) {
	if length > 64 || length < 0 {
		return NibblePath{}, fmt.Errorf("erroneous length")
	}

	if len(data) != (length+1)/2 {
		return NibblePath{}, fmt.Errorf("length does not match provided data")
	}

	if length%2 != 0 && (data[len(data)-1]^0xF) != 0 {
		return NibblePath{}, fmt.Errorf("garbage bits at end of data")
	}

	path := NibblePath{
		length:  length,
		nibbles: [32]byte{},
	}
	copy(path.nibbles[:], data)

	return path, nil
}

func (path NibblePath) getNibble(pos int) (byte, error) {
	if pos > path.length || pos < 0 {
		return 0, fmt.Errorf("out of range nibble access")
	}

	b := path.nibbles[pos/2]
	shift := ((pos + 1) % 2) * 4
	return (b >> shift) & 0x0F, nil
}

func (path NibblePath) Iter() iter.Seq2[int, byte] {
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

func (path NibblePath) Bytes() []byte {
	if path.length == 0 {
		return nil
	}

	data := make([]byte, (path.length+1)/2)
	copy(data, path.nibbles[:])

	return data
}
