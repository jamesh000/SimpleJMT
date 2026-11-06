package jmt

import (
	"errors"
	"fmt"
	"math/bits"
)

// SparseMerkleLeafNode is the hash-relevant portion of a leaf node.
type SparseMerkleLeafNode struct {
	KeyHash   Hash
	ValueHash Hash
}

func (node *SparseMerkleLeafNode) Hash() Hash {
	return concatHash(leafType, node.KeyHash, node.ValueHash)
}

// SparseMerkleProof is an inclusion or non-inclusion proof. Siblings are
// ordered from the root level to the bottom level.
type SparseMerkleProof struct {
	// Leaf is the terminal leaf. It is nil when the proof terminates at an
	// empty subtree.
	Leaf *SparseMerkleLeafNode
	// Siblings contains the sibling hashes needed to reconstruct the root.
	Siblings []Hash
}

// GetWithProof returns the value hash and value-store index for key, along with
// an inclusion or non-inclusion proof. For non-inclusion, valueHash and
// valueIndex are nil.
func (jmt JellyfishMerkleTree) GetWithProof(version Version, key Hash) (*Hash, *ValueIndex, *SparseMerkleProof, error) {
	currentKey := NodeKey{Version: version}
	keyPath, err := NewNibblePath(len(key)*2, key[:])
	if err != nil {
		return nil, nil, nil, err
	}

	siblings := make([]Hash, 0, HashLen*2*4)

	// A leaf can live at depth HashLen*2 when keys differ only in the last
	// nibble, so visit depths 0 through HashLen*2 inclusive.
	for depth := 0; depth <= HashLen*2; depth++ {
		node, err := jmt.reader.GetNode(currentKey)
		if err != nil {
			if errors.Is(err, ErrNodeNotFound) && currentKey.NibblePath.length == 0 {
				return nil, nil, &SparseMerkleProof{Siblings: siblings}, nil
			}
			return nil, nil, nil, err
		}

		switch n := node.(type) {
		case *InternalNode:
			nibble, err := keyPath.getNibble(depth)
			if err != nil {
				return nil, nil, nil, err
			}

			nextKey, nodeSiblings, found, err := n.getChildWithSiblings(currentKey, nibble)
			if err != nil {
				return nil, nil, nil, err
			}
			siblings = append(siblings, nodeSiblings...)
			if !found {
				return nil, nil, &SparseMerkleProof{Siblings: siblings}, nil
			}
			currentKey = nextKey

		case *LeafNode:
			proof := &SparseMerkleProof{
				Leaf: &SparseMerkleLeafNode{
					KeyHash:   n.keyHash,
					ValueHash: n.valueHash,
				},
				Siblings: siblings,
			}
			if n.keyHash != key {
				return nil, nil, proof, nil
			}

			valueHash := n.valueHash
			valueIndex := n.valueIndex
			return &valueHash, &valueIndex, proof, nil
		}
	}

	return nil, nil, nil, fmt.Errorf("reached end of key without finding value, erroneous node somewhere in tree")
}

// getChildWithSiblings returns the next child node and the sibling hashes for
// one internal node. The returned child is not necessarily at the requested
// nibble: if a subtree contains only one direct leaf child, that leaf is
// returned so it can serve as the terminal leaf of a non-inclusion proof.
func (node *InternalNode) getChildWithSiblings(currentKey NodeKey, nibble byte) (NodeKey, []Hash, bool, error) {
	// A node with a single direct leaf child hashes to that leaf directly, so
	// it contributes no siblings of its own.
	if bits.OnesCount16(node.bitmap) == 1 && node.leaves != 0 {
		onlyChildIndex := byte(bits.TrailingZeros16(node.bitmap))
		child, ok := node.Children[onlyChildIndex]
		if !ok {
			return NodeKey{}, nil, false, fmt.Errorf("malformed child map missing child at %x", onlyChildIndex)
		}
		childPath, err := currentKey.NibblePath.Append(onlyChildIndex)
		if err != nil {
			return NodeKey{}, nil, false, err
		}
		return NodeKey{Version: child.version, NibblePath: childPath}, nil, true, nil
	}

	siblings := make([]Hash, 0, 4)

	for height := 3; height >= 0; height-- {
		width := 1 << height
		childHalfStart := int(nibble) & (0xFF << height)
		siblingHalfStart := childHalfStart ^ width

		siblings = append(siblings, node.merkleHash(siblingHalfStart, width))

		mask := uint16(((uint32(1) << width) - 1) << childHalfStart)
		rangeBitmap := node.bitmap & mask
		rangeLeaves := node.leaves & mask

		if rangeBitmap == 0 {
			return NodeKey{}, siblings, false, nil
		}

		if width == 1 || (bits.OnesCount16(rangeBitmap) == 1 && rangeLeaves != 0) {
			onlyChildIndex := byte(bits.TrailingZeros16(rangeBitmap))
			child, ok := node.Children[onlyChildIndex]
			if !ok {
				return NodeKey{}, nil, false, fmt.Errorf("malformed child map missing child at %x", onlyChildIndex)
			}

			childPath, err := currentKey.NibblePath.Append(onlyChildIndex)
			if err != nil {
				return NodeKey{}, nil, false, err
			}

			return NodeKey{
				Version:    child.version,
				NibblePath: childPath,
			}, siblings, true, nil
		}
	}

	return NodeKey{}, nil, false, fmt.Errorf("unreachable: failed to find child in internal node")
}

// Verify checks an inclusion proof when valueHash is non-nil, or a
// non-inclusion proof when valueHash is nil.
func (proof *SparseMerkleProof) Verify(root Hash, key Hash, valueHash *Hash) error {
	if proof == nil {
		return fmt.Errorf("nil proof")
	}
	if len(proof.Siblings) > HashLen*8 {
		return fmt.Errorf("proof has too many siblings: %d", len(proof.Siblings))
	}

	var currentHash Hash
	if proof.Leaf != nil {
		if valueHash != nil {
			if key != proof.Leaf.KeyHash {
				return fmt.Errorf("inclusion proof key mismatch: proof key %s, requested key %s", proof.Leaf.KeyHash, key)
			}
			if *valueHash != proof.Leaf.ValueHash {
				return fmt.Errorf("inclusion proof value hash mismatch")
			}
		} else {
			if key == proof.Leaf.KeyHash {
				return fmt.Errorf("expected non-inclusion proof, but key exists in proof")
			}
			if commonPrefixBits(key, proof.Leaf.KeyHash) < len(proof.Siblings) {
				return fmt.Errorf("invalid non-inclusion proof: key would not share the proof leaf's subtree")
			}
		}
		currentHash = proof.Leaf.Hash()
	} else {
		if valueHash != nil {
			return fmt.Errorf("expected inclusion proof, found non-inclusion proof")
		}
		currentHash = emptyPlaceholderHash
	}

	for i := len(proof.Siblings) - 1; i >= 0; i-- {
		sibling := proof.Siblings[i]
		if keyBit(key, i) {
			currentHash = concatHash(internalType, sibling, currentHash)
		} else {
			currentHash = concatHash(internalType, currentHash, sibling)
		}
	}

	if currentHash != root {
		return fmt.Errorf("root hash mismatch: got %s, want %s", currentHash, root)
	}

	return nil
}

func keyBit(key Hash, index int) bool {
	return (key[index/8]>>(7-index%8))&1 != 0
}

func commonPrefixBits(a Hash, b Hash) int {
	for i := 0; i < HashLen; i++ {
		x := a[i] ^ b[i]
		if x != 0 {
			return i*8 + bits.LeadingZeros8(x)
		}
	}
	return HashLen * 8
}
