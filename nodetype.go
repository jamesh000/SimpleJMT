package main

import (
	"fmt"

	pb "github.com/jamesh000/SimpleJMT/nodepb"
	"google.golang.org/protobuf/proto"
)

type NodeKey struct {
	version    uint64
	nibblePath string
}

type KeyHash [32]byte

type Node interface {
	SerializeNode() ([]byte, error)
}

type child struct {
	key  NodeKey
	hash []byte
}

type InternalNode struct {
	Bitmap   uint16
	Children []child
}

func (node InternalNode) SerializeNode() ([]byte, error) {
	childrenPb := make([]*pb.Child, len(node.Children))
	for i, child := range node.Children {
		childrenPb[i] = &pb.Child{
			Version:    child.key.version,
			Nibblepath: []byte(child.key.nibblePath),
			Hash:       child.hash,
		}
	}

	nodePb := &pb.Node{
		Body: &pb.Node_Internal{
			Internal: &pb.Node_InternalNode{
				Bitmap:   uint32(node.Bitmap),
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
	valueHash []byte
}

func (node LeafNode) SerializeNode() ([]byte, error) {
	nodePb := &pb.Node{
		Body: &pb.Node_Leaf{
			Leaf: &pb.Node_LeafNode{
				Id:   node.keyHash[:],
				Hash: node.valueHash,
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
		children := make([]child, len(body.Internal.Children))
		for i, childPb := range body.Internal.Children {
			children[i] = child{
				key: NodeKey{
					version:    childPb.Version,
					nibblePath: string(childPb.Nibblepath),
				},
				hash: childPb.Hash,
			}
		}

		return InternalNode{
				Bitmap:   uint16(body.Internal.Bitmap),
				Children: children,
			},
			nil
	case *pb.Node_Leaf:
		return LeafNode{
				keyHash:   KeyHash(body.Leaf.Id),
				valueHash: body.Leaf.Hash,
			},
			nil
	}

	return nil, fmt.Errorf("Invalid node type")
}
