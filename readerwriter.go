package jmt

import "errors"

// ErrNodeNotFound is returned by TreeReader.GetNode when a node does not exist.
// Callers must use this to distinguish an empty tree from a storage failure.
var ErrNodeNotFound = errors.New("node not found")

type Value []byte

type TreeReader interface {
	GetNode(NodeKey) (Node, error)
}

type TreeWriter interface {
	WriteNodeBatch(map[NodeKey]Node) error
}
