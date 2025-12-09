package tree

import (
	"encoding/binary"
	"errors"
	"fmt"

	"microprolly/pkg/types"
)

const (
	// Node type prefixes
	nodeTypeLeaf     = 0x01
	nodeTypeInternal = 0x02
)

var (
	// ErrCorruptedData is returned when deserialization fails
	ErrCorruptedData = errors.New("data corruption detected")
)

func init() {
	// Register serialization functions with types package
	types.SerializeLeafNodeFunc = SerializeLeafNode
	types.SerializeInternalNodeFunc = SerializeInternalNode
}

// SerializeLeafNode serializes a LeafNode to bytes using deterministic binary encoding
func SerializeLeafNode(node *types.LeafNode) ([]byte, error) {
	// Calculate total size needed
	size := 1 + 4 // node type + pair count
	for _, pair := range node.Pairs {
		size += 4 + len(pair.Key) + 4 + len(pair.Value)
	}

	buf := make([]byte, 0, size)

	// Write node type
	buf = append(buf, nodeTypeLeaf)

	// Write pair count (big-endian)
	pairCount := make([]byte, 4)
	binary.BigEndian.PutUint32(pairCount, uint32(len(node.Pairs)))
	buf = append(buf, pairCount...)

	// Write each pair
	for _, pair := range node.Pairs {
		// Write key length
		keyLen := make([]byte, 4)
		binary.BigEndian.PutUint32(keyLen, uint32(len(pair.Key)))
		buf = append(buf, keyLen...)

		// Write key
		buf = append(buf, pair.Key...)

		// Write value length
		valueLen := make([]byte, 4)
		binary.BigEndian.PutUint32(valueLen, uint32(len(pair.Value)))
		buf = append(buf, valueLen...)

		// Write value
		buf = append(buf, pair.Value...)
	}

	return buf, nil
}

// SerializeInternalNode serializes an InternalNode to bytes using deterministic binary encoding
func SerializeInternalNode(node *types.InternalNode) ([]byte, error) {
	// Calculate total size needed
	size := 1 + 4 // node type + child count
	for _, child := range node.Children {
		size += 4 + len(child.Key) + 32 // key length + key + hash (32 bytes)
	}

	buf := make([]byte, 0, size)

	// Write node type
	buf = append(buf, nodeTypeInternal)

	// Write child count (big-endian)
	childCount := make([]byte, 4)
	binary.BigEndian.PutUint32(childCount, uint32(len(node.Children)))
	buf = append(buf, childCount...)

	// Write each child reference
	for _, child := range node.Children {
		// Write key length
		keyLen := make([]byte, 4)
		binary.BigEndian.PutUint32(keyLen, uint32(len(child.Key)))
		buf = append(buf, keyLen...)

		// Write key
		buf = append(buf, child.Key...)

		// Write hash (32 bytes)
		buf = append(buf, child.Hash[:]...)
	}

	return buf, nil
}

// DeserializeNode deserializes bytes into a Node (either LeafNode or InternalNode)
func DeserializeNode(data []byte) (types.Node, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("%w: empty data", ErrCorruptedData)
	}

	nodeType := data[0]

	switch nodeType {
	case nodeTypeLeaf:
		return DeserializeLeafNode(data)
	case nodeTypeInternal:
		return DeserializeInternalNode(data)
	default:
		return nil, fmt.Errorf("%w: unknown node type %d", ErrCorruptedData, nodeType)
	}
}

// DeserializeLeafNode deserializes bytes into a LeafNode
func DeserializeLeafNode(data []byte) (*types.LeafNode, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("%w: insufficient data for leaf node", ErrCorruptedData)
	}

	pos := 0

	// Read node type
	if data[pos] != nodeTypeLeaf {
		return nil, fmt.Errorf("%w: expected leaf node type", ErrCorruptedData)
	}
	pos++

	// Read pair count
	if pos+4 > len(data) {
		return nil, fmt.Errorf("%w: insufficient data for pair count", ErrCorruptedData)
	}
	pairCount := binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Read pairs
	pairs := make([]types.KVPair, 0, pairCount)
	for i := uint32(0); i < pairCount; i++ {
		// Read key length
		if pos+4 > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for key length", ErrCorruptedData)
		}
		keyLen := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4

		// Read key
		if pos+int(keyLen) > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for key", ErrCorruptedData)
		}
		key := make([]byte, keyLen)
		copy(key, data[pos:pos+int(keyLen)])
		pos += int(keyLen)

		// Read value length
		if pos+4 > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for value length", ErrCorruptedData)
		}
		valueLen := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4

		// Read value
		if pos+int(valueLen) > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for value", ErrCorruptedData)
		}
		value := make([]byte, valueLen)
		copy(value, data[pos:pos+int(valueLen)])
		pos += int(valueLen)

		pairs = append(pairs, types.KVPair{Key: key, Value: value})
	}

	// Validate that we consumed all bytes (no trailing data)
	if pos != len(data) {
		return nil, fmt.Errorf("%w: unexpected trailing data (%d bytes remaining)", ErrCorruptedData, len(data)-pos)
	}

	return &types.LeafNode{Pairs: pairs}, nil
}

// DeserializeInternalNode deserializes bytes into an InternalNode
func DeserializeInternalNode(data []byte) (*types.InternalNode, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("%w: insufficient data for internal node", ErrCorruptedData)
	}

	pos := 0

	// Read node type
	if data[pos] != nodeTypeInternal {
		return nil, fmt.Errorf("%w: expected internal node type", ErrCorruptedData)
	}
	pos++

	// Read child count
	if pos+4 > len(data) {
		return nil, fmt.Errorf("%w: insufficient data for child count", ErrCorruptedData)
	}
	childCount := binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Read children
	children := make([]types.ChildRef, 0, childCount)
	for i := uint32(0); i < childCount; i++ {
		// Read key length
		if pos+4 > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for key length", ErrCorruptedData)
		}
		keyLen := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4

		// Read key
		if pos+int(keyLen) > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for key", ErrCorruptedData)
		}
		key := make([]byte, keyLen)
		copy(key, data[pos:pos+int(keyLen)])
		pos += int(keyLen)

		// Read hash (32 bytes)
		if pos+32 > len(data) {
			return nil, fmt.Errorf("%w: insufficient data for hash", ErrCorruptedData)
		}
		var hash types.Hash
		copy(hash[:], data[pos:pos+32])
		pos += 32

		children = append(children, types.ChildRef{Key: key, Hash: hash})
	}

	// Validate that we consumed all bytes (no trailing data)
	if pos != len(data) {
		return nil, fmt.Errorf("%w: unexpected trailing data (%d bytes remaining)", ErrCorruptedData, len(data)-pos)
	}

	return &types.InternalNode{Children: children}, nil
}
