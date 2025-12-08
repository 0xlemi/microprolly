package types

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash represents a SHA-256 hash (32 bytes)
type Hash [32]byte

// String returns the hex-encoded string representation of the hash
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// HashFromBytes creates a Hash from a byte slice
func HashFromBytes(data []byte) Hash {
	return sha256.Sum256(data)
}

// KVPair represents a key-value pair
type KVPair struct {
	Key   []byte
	Value []byte
}

// Node represents a node in the Prolly Tree
type Node interface {
	// Hash returns the content hash of this node
	Hash() Hash
	// Serialize converts the node to bytes
	Serialize() ([]byte, error)
	// IsLeaf returns true for leaf nodes
	IsLeaf() bool
}

// LeafNode contains actual key-value pairs
type LeafNode struct {
	Pairs []KVPair
}

// IsLeaf returns true for leaf nodes
func (n *LeafNode) IsLeaf() bool {
	return true
}

// Hash computes the content hash of the leaf node
func (n *LeafNode) Hash() Hash {
	data, err := n.Serialize()
	if err != nil {
		// In practice, serialization should not fail for valid nodes
		panic(err)
	}
	return HashFromBytes(data)
}

// Serialize converts the leaf node to bytes
func (n *LeafNode) Serialize() ([]byte, error) {
	// Placeholder - will be implemented in tree package
	return nil, nil
}

// InternalNode contains references to child nodes
type InternalNode struct {
	Children []ChildRef
}

// IsLeaf returns false for internal nodes
func (n *InternalNode) IsLeaf() bool {
	return false
}

// Hash computes the content hash of the internal node
func (n *InternalNode) Hash() Hash {
	data, err := n.Serialize()
	if err != nil {
		// In practice, serialization should not fail for valid nodes
		panic(err)
	}
	return HashFromBytes(data)
}

// Serialize converts the internal node to bytes
func (n *InternalNode) Serialize() ([]byte, error) {
	// Placeholder - will be implemented in tree package
	return nil, nil
}

// ChildRef represents a reference to a child node
type ChildRef struct {
	Key  []byte
	Hash Hash
}

// Commit represents a snapshot of the database
type Commit struct {
	RootHash  Hash   `json:"root_hash"`
	Message   string `json:"message"`
	Parent    Hash   `json:"parent"`
	Timestamp int64  `json:"timestamp"`
}
