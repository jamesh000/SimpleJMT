package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const HashLen = 32

type Hash [HashLen]byte

func (h Hash) String() string {
	return base64.StdEncoding.EncodeToString(h[:])
}

func (h *Hash) FromBytes(data []byte) (*Hash, error) {
	if len(data) != DIGEST_LEN {
		return nil, fmt.Errorf("Hash is wrong length")
	}

	copy(h[:], data)

	return h, nil
}

func NewHash(bytes []byte) Hash {
	return sha256.Sum256(bytes)
}
