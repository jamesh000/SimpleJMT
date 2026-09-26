package jmt

import (
	"fmt"
	"math/bits"
	"slices"

	pb "github.com/jamesh000/SimpleJMT/nodepb"
	"google.golang.org/protobuf/proto"
)

type NodeKey struct {
	Version    Version
	NibblePath NibblePath
}

type KeyHash = Hash

type Node interface {
	SerializeNode() ([]byte, error)
}

type child struct {
	version Version
	hash    []byte
}

type InternalNode struct {
	Children map[byte]child
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
				ValueHash: currentChild.hash,
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
	keyHash   KeyHash
	valueHash Hash
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
				children[i] = child{
					version: body.Internal.Children[currentChild].Version,
					hash:    body.Internal.Children[currentChild].ValueHash,
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

		keyHash := KeyHash{}
		copy(keyHash[:], body.Leaf.Id)

		return LeafNode{
				keyHash:   keyHash,
				valueHash: *valueHash,
			},
			nil
	}

	return nil, fmt.Errorf("Invalid node type")
}
