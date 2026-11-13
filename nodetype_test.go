package jmt

import (
	"reflect"
	"testing"

	pb "github.com/jamesh000/SimpleJMT/nodepb"
	"google.golang.org/protobuf/proto"
)

func TestInternalNodeSerializationRoundTrip(t *testing.T) {
	leaf1 := NewLeafNode(hashByte(0x01), hashByte(0x11), ValueIndex{KeyHash: hashByte(0x01), Version: 1})
	leaf2 := NewLeafNode(hashByte(0x02), hashByte(0x22), ValueIndex{KeyHash: hashByte(0x02), Version: 2})

	childInternal := NewInternalNode()
	childInternal.insertChild(0x3, 3, leaf2.Hash(), leafType)

	node := NewInternalNode()
	node.insertChild(0x0, 1, leaf1.Hash(), leafType)
	node.insertChild(0x5, 2, leaf2.Hash(), leafType)
	node.insertChild(0xf, 3, childInternal.Hash(), internalType)

	data, err := node.SerializeNode()
	if err != nil {
		t.Fatalf("SerializeNode() error = %v", err)
	}

	decoded, err := DeserializeNode(data)
	if err != nil {
		t.Fatalf("DeserializeNode() error = %v", err)
	}

	got, ok := decoded.(*InternalNode)
	if !ok {
		t.Fatalf("DeserializeNode() returned %T, want *InternalNode", decoded)
	}
	if !reflect.DeepEqual(got, node) {
		t.Fatalf("DeserializeNode() = %+v, want %+v", got, node)
	}
	if got.Hash() != node.Hash() {
		t.Fatalf("deserialized hash = %s, want %s", got.Hash(), node.Hash())
	}
}

func TestLeafHashIgnoresValueIndex(t *testing.T) {
	keyHash := hashByte(0x01)
	valueHash := hashByte(0x11)

	leaf1 := NewLeafNode(keyHash, valueHash, ValueIndex{KeyHash: keyHash, Version: 1})
	leaf2 := NewLeafNode(keyHash, valueHash, ValueIndex{KeyHash: keyHash, Version: 99})

	if leaf1.Hash() != leaf2.Hash() {
		t.Fatalf("leaf hashes differ despite only value index changing")
	}
	if leaf1.valueIndex == leaf2.valueIndex {
		t.Fatal("test leaves unexpectedly have equal value indexes")
	}
}

func TestInternalNodeCopyIsIndependent(t *testing.T) {
	leaf1 := NewLeafNode(hashByte(0x01), hashByte(0x11), ValueIndex{KeyHash: hashByte(0x01), Version: 1})
	leaf2 := NewLeafNode(hashByte(0x02), hashByte(0x22), ValueIndex{KeyHash: hashByte(0x02), Version: 2})

	node := NewInternalNode()
	node.insertChild(0x0, 1, leaf1.Hash(), leafType)

	clone := node.Copy()
	clone.insertChild(0x1, 2, leaf2.Hash(), leafType)

	if _, ok := node.Children[0x1]; ok {
		t.Fatal("modifying the copy changed the original node")
	}
	if _, ok := clone.Children[0x1]; !ok {
		t.Fatal("copy is missing the inserted child")
	}
}

func TestDeserializeNodeRejectsMalformedData(t *testing.T) {
	valueHash := hashByte(0x11)
	valueIndexKeyHash := hashByte(0x01)

	malformedLeaf := &pb.Node{
		Body: &pb.Node_Leaf{
			Leaf: &pb.Node_LeafNode{
				Id:                []byte{0x01},
				ValueHash:         valueHash[:],
				ValueIndexKeyHash: valueIndexKeyHash[:],
			},
		},
	}
	leafData, err := proto.Marshal(malformedLeaf)
	if err != nil {
		t.Fatalf("proto.Marshal(leaf) error = %v", err)
	}

	oversizedBitmap := &pb.Node{
		Body: &pb.Node_Internal{
			Internal: &pb.Node_InternalNode{
				Bitmap: 1 << 16,
			},
		},
	}
	bitmapData, err := proto.Marshal(oversizedBitmap)
	if err != nil {
		t.Fatalf("proto.Marshal(bitmap) error = %v", err)
	}

	invalidLeaves := &pb.Node{
		Body: &pb.Node_Internal{
			Internal: &pb.Node_InternalNode{
				Bitmap: 1,
				Leaves: 2,
			},
		},
	}
	leavesData, err := proto.Marshal(invalidLeaves)
	if err != nil {
		t.Fatalf("proto.Marshal(leaves) error = %v", err)
	}

	shortChildHash := &pb.Node{
		Body: &pb.Node_Internal{
			Internal: &pb.Node_InternalNode{
				Bitmap: 1,
				Leaves: 1,
				Children: []*pb.Child{
					{
						Version:   1,
						ValueHash: []byte{0x01},
						Type:      uint32(leafType),
					},
				},
			},
		},
	}
	childData, err := proto.Marshal(shortChildHash)
	if err != nil {
		t.Fatalf("proto.Marshal(child) error = %v", err)
	}

	for name, data := range map[string][]byte{
		"empty":            nil,
		"short leaf key":   leafData,
		"oversized bitmap": bitmapData,
		"invalid leaves":   leavesData,
		"short child hash": childData,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DeserializeNode(data); err == nil {
				t.Fatal("DeserializeNode() succeeded, want error")
			}
		})
	}

}
