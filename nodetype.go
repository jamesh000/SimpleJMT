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
	leafType     nodeType = 0x00
	internalType nodeType = 0x01
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
}

func concatHash(nt nodeType, left Hash, right Hash) Hash {
	concat := make([]byte, 65)
	concat[0] = byte(nt)
	copy(concat[1:33], left[:])
	copy(concat[33:65], right[:])
	return NewHash(concat)
}

func (node InternalNode) Hash() Hash {
	tree := make([]*child, 32)
	for i := byte(0); i <= 0xF; i++ {
		child, ok := node.Children[i]
		if ok {
			tree[16+i] = &child
		} else {
			tree[16+i] = nil
		}
	}

	for j := 8; ; j /= 2 {
		for i := j; i < 2*j; i++ {
			leftChild := tree[2*i]
			rightChild := tree[2*i+1]
			if leftChild != nil && rightChild != nil {
				tree[i] = &child{
					hash:     concatHash(internalType, leftChild.hash, rightChild.hash),
					nodeType: internalType,
				}
			} else if leftChild != nil {
				if leftChild.nodeType == leafType {
					tree[i] = leftChild
				} else {
					tree[i] = &child{
						hash:     concatHash(internalType, leftChild.hash, emptyPlaceholderHash),
						nodeType: internalType,
					}
				}
			} else if rightChild != nil {
				if rightChild.nodeType == leafType {
					tree[i] = rightChild
				} else {
					tree[i] = &child{
						hash:     concatHash(internalType, emptyPlaceholderHash, rightChild.hash),
						nodeType: internalType,
					}
				}
			} else {
				tree[i] = nil
			}
		}
		if j == 1 {
			break
		}
	}

	root := tree[1]

	if root == nil {
		panic(fmt.Errorf("internal node has no children"))
	}

	return root.hash
}

func (node InternalNode) Copy() InternalNode {
	//clone := new(InternalNode)
	//clone.Children = maps.Clone(node.Children)
	return InternalNode{
		Children: maps.Clone(node.Children),
	}
}

func (node InternalNode) SerializeNode() ([]byte, error) {
	bitmap := uint16(0)
	nibbles := make([]byte, 0, 16)
	for nibble := range node.Children {
		if nibble >= 16 {
			return nil, fmt.Errorf("invalid nibble in map")
		}

		nibbles = append(nibbles, nibble)
		bitmap |= 1 << nibble
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
				Bitmap:   uint32(bitmap),
				Children: childrenPb,
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
	keyHash   Hash
	valueHash Hash
}

func (node LeafNode) Hash() Hash {
	return concatHash(leafType, node.keyHash, node.valueHash)
}

func (node LeafNode) SerializeNode() ([]byte, error) {
	nodePb := &pb.Node{
		Body: &pb.Node_Leaf{
			Leaf: &pb.Node_LeafNode{
				Id:        node.keyHash[:],
				ValueHash: node.valueHash[:],
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
		if len(body.Internal.Children) != bits.OnesCount32(body.Internal.Bitmap) {
			return nil, fmt.Errorf("bitmap does not match number of children")
		}

		children := make(map[byte]child)
		currentChild := 0
		for i := byte(0); i < 16; i++ {
			if body.Internal.Bitmap&(1<<i) != 0 {
				var hash Hash
				copy(hash[:], body.Internal.Children[currentChild].ValueHash)

				children[i] = child{
					version:  body.Internal.Children[currentChild].Version,
					hash:     hash,
					nodeType: nodeType(body.Internal.Children[currentChild].Type),
				}
				currentChild++
			}
		}

		return InternalNode{children}, nil
	case *pb.Node_Leaf:
		valueHash, err := new(Hash).FromBytes(body.Leaf.ValueHash)
		if err != nil {
			return nil, err
		}

		keyHash := Hash{}
		copy(keyHash[:], body.Leaf.Id)

		return LeafNode{
				keyHash:   keyHash,
				valueHash: *valueHash,
			},
			nil
	}

	return nil, fmt.Errorf("Invalid node type")
}
