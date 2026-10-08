package jmt

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"iter"
)

const nibblePathMax = 2 * HashLen
const nibbleStorageLen = (nibblePathMax + 1) / 2

type NibblePath struct {
	length  int
	nibbles [nibbleStorageLen]byte
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

func (path *NibblePath) Append(nibble byte) error {
	if path.length+1 > nibblePathMax {
		return fmt.Errorf("Tried to append to full nibble path")
	}

	if (nibble & 0xF0) != 0 {
		return fmt.Errorf("Invalid nibble format for appending")
	}

	if path.length%2 == 0 {
		path.nibbles[path.length/2] = nibble << 4
	} else {
		path.nibbles[path.length/2] |= nibble
	}

	path.length++

	return nil
}

func (path *NibblePath) Bytes() []byte {
	buf := bytes.Buffer{}

	if path.length == 0 {
		return nil
	}

	binary.Write(&buf, binary.BigEndian, path.length)
	buf.Write(path.nibbles[:(path.length+1)/2])

	return buf.Bytes()
}

func (path *NibblePath) FromBytes(data []byte) (*NibblePath, error) {
	r := bytes.NewReader(data)

	if err := binary.Read(r, binary.BigEndian, &path.length); err != nil {
		return nil, err
	}

	if r.Len() != (path.length+1)/2 {
		return nil, fmt.Errorf("nibblepath length does not match actual size")
	}

	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if path.length%2 != 0 && bytes[len(bytes)-1]&0xF != 0 {
		return nil, fmt.Errorf("garbage bits at end of bytes")
	}

	copy(path.nibbles[:], bytes)

	return path, nil
}

func (path NibblePath) String() string {
	s := hex.EncodeToString(path.nibbles[:])
	return s[:path.length]
}
