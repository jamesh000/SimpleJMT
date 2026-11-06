package jmt

import (
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble"
)

const pebbleNodeKeyPrefix byte = 0x00

// PebbleDB implements TreeReader and TreeWriter using a Pebble database.
type PebbleDB struct {
	db *pebble.DB
}

var _ TreeReader = (*PebbleDB)(nil)
var _ TreeWriter = (*PebbleDB)(nil)

func NewPebbleDB(path string) (*PebbleDB, error) {
	return NewPebbleDBWithOptions(path, nil)
}

func NewPebbleDBWithOptions(path string, opts *pebble.Options) (*PebbleDB, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}

	return &PebbleDB{db: db}, nil
}

func (store *PebbleDB) Close() error {
	return store.db.Close()
}

func (store *PebbleDB) GetNode(key NodeKey) (Node, error) {
	encodedKey, err := encodePebbleNodeKey(key)
	if err != nil {
		return nil, err
	}

	data, closer, err := store.db.Get(encodedKey)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, fmt.Errorf("%w: version %d path %s", ErrNodeNotFound, key.Version, key.NibblePath.String())
		}
		return nil, err
	}
	defer closer.Close()

	// The value returned by Pebble is only valid until the closer is closed.
	nodeData := append([]byte(nil), data...)
	return DeserializeNode(nodeData)
}

func (store *PebbleDB) WriteNodeBatch(batch map[NodeKey]Node) error {
	wb := store.db.NewBatch()
	defer wb.Close()

	for key, node := range batch {
		encodedKey, err := encodePebbleNodeKey(key)
		if err != nil {
			return err
		}

		data, err := node.SerializeNode()
		if err != nil {
			return err
		}

		if err := wb.Set(encodedKey, data, nil); err != nil {
			return err
		}
	}

	return store.db.Apply(wb, pebble.Sync)
}

func encodePebbleNodeKey(key NodeKey) ([]byte, error) {
	encoded, err := key.Encode()
	if err != nil {
		return nil, err
	}

	result := make([]byte, 1+len(encoded))
	result[0] = pebbleNodeKeyPrefix
	copy(result[1:], encoded)
	return result, nil
}
