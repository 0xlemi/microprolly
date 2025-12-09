package tree

import (
	"bytes"
	"testing"

	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// Generators for property-based testing

// genKVPair generates a random KVPair
func genKVPair() *rapid.Generator[types.KVPair] {
	return rapid.Custom(func(t *rapid.T) types.KVPair {
		key := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(t, "key")
		value := rapid.SliceOf(rapid.Byte()).Draw(t, "value")
		return types.KVPair{Key: key, Value: value}
	})
}

// genLeafNode generates a random LeafNode
func genLeafNode() *rapid.Generator[*types.LeafNode] {
	return rapid.Custom(func(t *rapid.T) *types.LeafNode {
		pairs := rapid.SliceOfN(genKVPair(), 1, 20).Draw(t, "pairs")
		return &types.LeafNode{Pairs: pairs}
	})
}

// genHash generates a random Hash
func genHash() *rapid.Generator[types.Hash] {
	return rapid.Custom(func(t *rapid.T) types.Hash {
		var hash types.Hash
		bytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "hash_bytes")
		copy(hash[:], bytes)
		return hash
	})
}

// genChildRef generates a random ChildRef
func genChildRef() *rapid.Generator[types.ChildRef] {
	return rapid.Custom(func(t *rapid.T) types.ChildRef {
		key := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(t, "key")
		hash := genHash().Draw(t, "hash")
		return types.ChildRef{Key: key, Hash: hash}
	})
}

// genInternalNode generates a random InternalNode
func genInternalNode() *rapid.Generator[*types.InternalNode] {
	return rapid.Custom(func(t *rapid.T) *types.InternalNode {
		children := rapid.SliceOfN(genChildRef(), 1, 20).Draw(t, "children")
		return &types.InternalNode{Children: children}
	})
}

// TestProperty_NodeSerializationDeterminism tests Property 17: Node Serialization Determinism
// **Feature: versioned-kv-store, Property 17: Node Serialization Determinism**
// **Validates: Requirements 10.1, 10.2, 10.4**
//
// For any node (leaf or internal), serializing it multiple times SHALL produce identical byte sequences.
func TestProperty_NodeSerializationDeterminism(t *testing.T) {
	t.Run("LeafNode", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			node := genLeafNode().Draw(t, "leaf_node")

			// Serialize the node twice
			data1, err1 := SerializeLeafNode(node)
			if err1 != nil {
				t.Fatalf("First serialization failed: %v", err1)
			}

			data2, err2 := SerializeLeafNode(node)
			if err2 != nil {
				t.Fatalf("Second serialization failed: %v", err2)
			}

			// Verify determinism: both serializations should be identical
			if !bytes.Equal(data1, data2) {
				t.Fatalf("Determinism failed: serializations differ\nFirst:  %x\nSecond: %x", data1, data2)
			}
		})
	})

	t.Run("InternalNode", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			node := genInternalNode().Draw(t, "internal_node")

			// Serialize the node twice
			data1, err1 := SerializeInternalNode(node)
			if err1 != nil {
				t.Fatalf("First serialization failed: %v", err1)
			}

			data2, err2 := SerializeInternalNode(node)
			if err2 != nil {
				t.Fatalf("Second serialization failed: %v", err2)
			}

			// Verify determinism: both serializations should be identical
			if !bytes.Equal(data1, data2) {
				t.Fatalf("Determinism failed: serializations differ\nFirst:  %x\nSecond: %x", data1, data2)
			}
		})
	})
}

// TestProperty_NodeSerializationRoundTrip tests Property 18: Node Serialization Round-Trip
// **Feature: versioned-kv-store, Property 18: Node Serialization Round-Trip**
// **Validates: Requirements 10.3**
//
// For any node (leaf or internal), serializing then deserializing SHALL produce an equivalent node with identical content.
func TestProperty_NodeSerializationRoundTrip(t *testing.T) {
	t.Run("LeafNode", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genLeafNode().Draw(t, "leaf_node")

			// Serialize the node
			data, err := SerializeLeafNode(original)
			if err != nil {
				t.Fatalf("Serialization failed: %v", err)
			}

			// Deserialize the node
			deserialized, err := DeserializeLeafNode(data)
			if err != nil {
				t.Fatalf("Deserialization failed: %v", err)
			}

			// Verify round-trip: nodes should be equivalent
			if len(original.Pairs) != len(deserialized.Pairs) {
				t.Fatalf("Round-trip failed: pair count mismatch, got %d, want %d", len(deserialized.Pairs), len(original.Pairs))
			}

			for i := range original.Pairs {
				if !bytes.Equal(original.Pairs[i].Key, deserialized.Pairs[i].Key) {
					t.Fatalf("Round-trip failed: key mismatch at index %d", i)
				}
				if !bytes.Equal(original.Pairs[i].Value, deserialized.Pairs[i].Value) {
					t.Fatalf("Round-trip failed: value mismatch at index %d", i)
				}
			}
		})
	})

	t.Run("InternalNode", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genInternalNode().Draw(t, "internal_node")

			// Serialize the node
			data, err := SerializeInternalNode(original)
			if err != nil {
				t.Fatalf("Serialization failed: %v", err)
			}

			// Deserialize the node
			deserialized, err := DeserializeInternalNode(data)
			if err != nil {
				t.Fatalf("Deserialization failed: %v", err)
			}

			// Verify round-trip: nodes should be equivalent
			if len(original.Children) != len(deserialized.Children) {
				t.Fatalf("Round-trip failed: child count mismatch, got %d, want %d", len(deserialized.Children), len(original.Children))
			}

			for i := range original.Children {
				if !bytes.Equal(original.Children[i].Key, deserialized.Children[i].Key) {
					t.Fatalf("Round-trip failed: key mismatch at index %d", i)
				}
				if original.Children[i].Hash != deserialized.Children[i].Hash {
					t.Fatalf("Round-trip failed: hash mismatch at index %d", i)
				}
			}
		})
	})

	t.Run("DeserializeNode", func(t *testing.T) {
		// Test the generic DeserializeNode function with both node types
		rapid.Check(t, func(t *rapid.T) {
			// Randomly choose between leaf and internal node
			isLeaf := rapid.Bool().Draw(t, "is_leaf")

			if isLeaf {
				original := genLeafNode().Draw(t, "leaf_node")
				data, err := SerializeLeafNode(original)
				if err != nil {
					t.Fatalf("Serialization failed: %v", err)
				}

				node, err := DeserializeNode(data)
				if err != nil {
					t.Fatalf("DeserializeNode failed: %v", err)
				}

				if !node.IsLeaf() {
					t.Fatal("DeserializeNode returned non-leaf node for leaf data")
				}

				deserialized := node.(*types.LeafNode)
				if len(original.Pairs) != len(deserialized.Pairs) {
					t.Fatalf("Round-trip failed: pair count mismatch, got %d, want %d", len(deserialized.Pairs), len(original.Pairs))
				}
			} else {
				original := genInternalNode().Draw(t, "internal_node")
				data, err := SerializeInternalNode(original)
				if err != nil {
					t.Fatalf("Serialization failed: %v", err)
				}

				node, err := DeserializeNode(data)
				if err != nil {
					t.Fatalf("DeserializeNode failed: %v", err)
				}

				if node.IsLeaf() {
					t.Fatal("DeserializeNode returned leaf node for internal data")
				}

				deserialized := node.(*types.InternalNode)
				if len(original.Children) != len(deserialized.Children) {
					t.Fatalf("Round-trip failed: child count mismatch, got %d, want %d", len(deserialized.Children), len(original.Children))
				}
			}
		})
	})
}
