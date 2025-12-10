package chunker

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
		key := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "key")
		value := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "value")
		return types.KVPair{Key: key, Value: value}
	})
}

// genKVPairs generates a slice of random KVPairs
func genKVPairs() *rapid.Generator[[]types.KVPair] {
	return rapid.SliceOfN(genKVPair(), 0, 50)
}

// TestProperty_KVPairSerializationDeterminism tests Property 4: KV Pair Serialization Determinism
// **Feature: versioned-kv-store, Property 4: KV Pair Serialization Determinism**
// **Validates: Requirements 2.4**
//
// For any set of KV pairs, serializing them multiple times SHALL produce identical byte sequences.
func TestProperty_KVPairSerializationDeterminism(t *testing.T) {
	t.Run("SinglePair", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			pair := genKVPair().Draw(t, "pair")

			// Serialize the pair twice
			data1 := SerializeKVPair(pair)
			data2 := SerializeKVPair(pair)

			// Verify determinism: both serializations should be identical
			if !bytes.Equal(data1, data2) {
				t.Fatalf("Determinism failed: serializations differ\nFirst:  %x\nSecond: %x", data1, data2)
			}
		})
	})

	t.Run("MultiplePairs", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			pairs := genKVPairs().Draw(t, "pairs")

			// Serialize the pairs twice
			data1 := SerializeKVPairs(pairs)
			data2 := SerializeKVPairs(pairs)

			// Verify determinism: both serializations should be identical
			if !bytes.Equal(data1, data2) {
				t.Fatalf("Determinism failed: serializations differ\nFirst:  %x\nSecond: %x", data1, data2)
			}
		})
	})
}

// TestProperty_KVPairSerializationRoundTrip tests Property 5: KV Pair Serialization Round-Trip
// **Feature: versioned-kv-store, Property 5: KV Pair Serialization Round-Trip**
// **Validates: Requirements 2.5**
//
// For any set of KV pairs, serializing then deserializing SHALL produce an equivalent set of pairs.
func TestProperty_KVPairSerializationRoundTrip(t *testing.T) {
	t.Run("SinglePair", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genKVPair().Draw(t, "pair")

			// Serialize the pair
			data := SerializeKVPair(original)

			// Deserialize the pair
			deserialized, consumed, err := DeserializeKVPair(data)
			if err != nil {
				t.Fatalf("Deserialization failed: %v", err)
			}

			// Verify all bytes were consumed
			if consumed != len(data) {
				t.Fatalf("Not all bytes consumed: consumed %d, total %d", consumed, len(data))
			}

			// Verify round-trip: pairs should be equivalent
			if !bytes.Equal(original.Key, deserialized.Key) {
				t.Fatalf("Round-trip failed: key mismatch\nOriginal: %x\nDeserialized: %x", original.Key, deserialized.Key)
			}
			if !bytes.Equal(original.Value, deserialized.Value) {
				t.Fatalf("Round-trip failed: value mismatch\nOriginal: %x\nDeserialized: %x", original.Value, deserialized.Value)
			}
		})
	})

	t.Run("MultiplePairs", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := genKVPairs().Draw(t, "pairs")

			// Serialize the pairs
			data := SerializeKVPairs(original)

			// Deserialize the pairs
			deserialized, err := DeserializeKVPairs(data)
			if err != nil {
				t.Fatalf("Deserialization failed: %v", err)
			}

			// Verify round-trip: pair count should match
			if len(original) != len(deserialized) {
				t.Fatalf("Round-trip failed: pair count mismatch, got %d, want %d", len(deserialized), len(original))
			}

			// Verify each pair
			for i := range original {
				if !bytes.Equal(original[i].Key, deserialized[i].Key) {
					t.Fatalf("Round-trip failed: key mismatch at index %d\nOriginal: %x\nDeserialized: %x", i, original[i].Key, deserialized[i].Key)
				}
				if !bytes.Equal(original[i].Value, deserialized[i].Value) {
					t.Fatalf("Round-trip failed: value mismatch at index %d\nOriginal: %x\nDeserialized: %x", i, original[i].Value, deserialized[i].Value)
				}
			}
		})
	})
}
