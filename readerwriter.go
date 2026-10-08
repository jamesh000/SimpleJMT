package jmt

type Value []byte

type TreeReader interface {
	GetNode(NodeKey) (Node, error)
	GetValue(Hash, Version) (Value, error)
}

type TreeWriter interface {
	WriteNodeBatch(map[NodeKey]Node) error
}
