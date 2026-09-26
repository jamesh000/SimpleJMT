package jmt

type Value []byte

type TreeReader interface {
	GetNode(NodeKey) (Node, error)
	GetValue(KeyHash) (Value, error)
}

type TreeWriter interface {
	WriteNodeBatch(map[NodeKey]Node) error
}
