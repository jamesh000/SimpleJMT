package jmt

// Children need to record their type as well, which is used when hashing internal nodes
// Leaf children are collapsed upward, but internal children are hashed against the placeholder

import (
	"fmt"
	"maps"
	"math/bits"
	"slices"

	pb "github.com/jamesh000/SimpleJMT/nodepb"
	"google.golang.org/protobuf/proto"
)

type nodeType byte

const (
	invalidType  nodeType = 0x00
	leafType     nodeType = 0x01
	internalType nodeType = 0x02
)

type NodeKey struct {
	Version    Version
	NibblePath NibblePath
}

type Node interface {
	SerializeNode() ([]byte, error)
	Hash() Hash
}

type child struct {
	version  Version
	hash     Hash
	nodeType nodeType
}

type InternalNode struct {
	Children map[byte]child
	bitmap   uint16
	leaves   uint16
}

// Create empty new internal node
func NewInternalNode() *InternalNode {
	return &InternalNode{
		Children: make(map[byte]child),
		bitmap:   uint16(0),
		leaves:   uint16(0),
	}
}

// Overwrites old child, but will likely never be run on anything but a new internal node
// that either:
// 1. Is totally blank
// 2. Is a copy of an older internal node that is being updated
func (node *InternalNode) insertChild(nibble byte, version Version, hash Hash, nodeType nodeType) {
	if nibble >= 16 {
		panic(fmt.Errorf("Trying to insert child at nibble that doesn't exist"))
	}

	node.bitmap |= 1 << nibble
	if nodeType == leafType {
		node.leaves |= 1 << nibble
	} else {
		node.leaves &^= 1 << nibble
	}

	node.Children[nibble] = child{
		version:  version,
		hash:     hash,
		nodeType: nodeType,
	}
}

func concatHash(nt nodeType, left Hash, right Hash) Hash {
	concat := make([]byte, 65)
	concat[0] = byte(nt)
	copy(concat[1:33], left[:])
	copy(concat[33:65], right[:])
	return NewHash(concat)
}

func (node *InternalNode) Hash() Hash {
	return node.merkleHash(0, 16)
}

func (node *InternalNode) merkleHash(pos int, width int) Hash {
	mask := uint16(((uint32(1) << width) - 1) << pos)
	bitmap, leaves := node.bitmap&mask, node.leaves&mask
	if bitmap == 0 {
		return emptyPlaceholderHash
	} else if width == 1 || (bits.OnesCount16(bitmap) == 1 && leaves != 0) {
		nibble := byte(bits.TrailingZeros16(bitmap))
		child, ok := node.Children[nibble]
		if !ok {
			panic(fmt.Errorf("Malformed child map missing child at %x", nibble))
		}
		return child.hash
	} else {
		half := width / 2
		left := node.merkleHash(pos, half)
		right := node.merkleHash(pos+half, half)

		return concatHash(internalType, left, right)
	}
}

func (node *InternalNode) Copy() *InternalNode {
	//clone := new(InternalNode)
	//clone.Children = maps.Clone(node.Children)
	return &InternalNode{
		Children: maps.Clone(node.Children),
		bitmap:   node.bitmap,
		leaves:   node.leaves,
	}
}

func (node *InternalNode) SerializeNode() ([]byte, error) {
	nibbles := make([]byte, 0, 16)
	for nibble := range node.Children {
		if nibble >= 16 {
			return nil, fmt.Errorf("invalid nibble in map")
		}

		nibbles = append(nibbles, nibble)
	}
	slices.Sort(nibbles)

	childrenPb := make([]*pb.Child, 0, len(nibbles))
	for _, nibble := range nibbles {
		currentChild := node.Children[nibble]
		childrenPb = append(childrenPb,
			&pb.Child{
				Version:   currentChild.version,
				ValueHash: currentChild.hash[:],
				Type:      uint32(currentChild.nodeType),
			})
	}

	nodePb := &pb.Node{
		Body: &pb.Node_Internal{
			Internal: &pb.Node_InternalNode{
				Children: childrenPb,
				Bitmap:   uint32(node.bitmap),
				Leaves:   uint32(node.leaves),
			},
		},
	}

	data, err := proto.Marshal(nodePb)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type LeafNode struct {
	keyHash    Hash
	valueHash  Hash
	valueIndex ValueIndex
}

func NewLeafNode(keyHash Hash, valueHash Hash, valueIndex ValueIndex) *LeafNode {
	return &LeafNode{
		keyHash:    keyHash,
		valueHash:  valueHash,
		valueIndex: valueIndex,
	}
}

// Hash commits to the key and value hashes only. The value index is metadata
// used to locate the value in the separate versioned value store.
func (node *LeafNode) Hash() Hash {
	return concatHash(leafType, node.keyHash, node.valueHash)
}

func (node *LeafNode) SerializeNode() ([]byte, error) {
	nodePb := &pb.Node{
		Body: &pb.Node_Leaf{
			Leaf: &pb.Node_LeafNode{
				Id:                node.keyHash[:],
				ValueHash:         node.valueHash[:],
				ValueIndexKeyHash: node.valueIndex.KeyHash[:],
				ValueIndexVersion: node.valueIndex.Version,
			},
		},
	}

	data, err := proto.Marshal(nodePb)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func DeserializeNode(data []byte) (Node, error) {
	nodePb := &pb.Node{}
	err := proto.Unmarshal(data, nodePb)
	if err != nil {
		return nil, err
	}

	switch body := nodePb.Body.(type) {
	case *pb.Node_Internal:
		if body.Internal == nil {
			return nil, fmt.Errorf("internal node body is nil")
		}
		if body.Internal.Bitmap > 0xFFFF || body.Internal.Leaves > 0xFFFF {
			return nil, fmt.Errorf("internal node bitmap exceeds 16 bits")
		}

		bitmap := uint16(body.Internal.Bitmap)
		leaves := uint16(body.Internal.Leaves)
		if leaves&^bitmap != 0 {
			return nil, fmt.Errorf("leaf bitmap is not a subset of child bitmap")
		}
		if len(body.Internal.Children) != bits.OnesCount16(bitmap) {
			return nil, fmt.Errorf("bitmap does not match number of children")
		}

		children := make(map[byte]child)
		currentChild := 0
		for i := byte(0); i < 16; i++ {
			if bitmap&(1<<i) != 0 {
				childPb := body.Internal.Children[currentChild]
				if childPb == nil {
					return nil, fmt.Errorf("internal node has nil child")
				}

				hash, err := new(Hash).FromBytes(childPb.ValueHash)
				if err != nil {
					return nil, err
				}

				if childPb.Type != uint32(leafType) && childPb.Type != uint32(internalType) {
					return nil, fmt.Errorf("invalid child type %d", childPb.Type)
				}
				childType := nodeType(childPb.Type)
				isLeaf := leaves&(1<<i) != 0
				if isLeaf != (childType == leafType) {
					return nil, fmt.Errorf("child type does not match leaf bitmap at nibble %x", i)
				}

				children[i] = child{
					version:  childPb.Version,
					hash:     *hash,
					nodeType: childType,
				}
				currentChild++
			}
		}

		return &InternalNode{
			Children: children,
			bitmap:   bitmap,
			leaves:   leaves,
		}, nil
	case *pb.Node_Leaf:
		if body.Leaf == nil {
			return nil, fmt.Errorf("leaf node body is nil")
		}

		keyHash, err := new(Hash).FromBytes(body.Leaf.Id)
		if err != nil {
			return nil, err
		}

		valueHash, err := new(Hash).FromBytes(body.Leaf.ValueHash)
		if err != nil {
			return nil, err
		}

		valueIndexKeyHash, err := new(Hash).FromBytes(body.Leaf.ValueIndexKeyHash)
		if err != nil {
			return nil, err
		}

		return NewLeafNode(
			*keyHash,
			*valueHash,
			ValueIndex{
				KeyHash: *valueIndexKeyHash,
				Version: body.Leaf.ValueIndexVersion,
			},
		), nil
	}

	return nil, fmt.Errorf("Invalid node type")
}
