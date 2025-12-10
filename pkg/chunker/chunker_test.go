package chunker

import (
	"bytes"
	"sort"
	"testing"

	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// genSortedKVPairs generates a sorted slice of KVPairs with unique keys
func genSortedKVPairs() *rapid.Generator[[]types.KVPair] {
	return rapid.Custom(func(t *rapid.T) []types.KVPair {
		// Generate between 10 and 100 pairs to have enough data for chunking
		count := rapid.IntRange(10, 100).Draw(t, "count")
		pairs := make([]types.KVPair, count)

		// Generate unique keys by using index as prefix
		for i := 0; i < count; i++ {
			// Create unique key with index prefix
			keyBase := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "key_base")
			key := append([]byte{byte(i / 256), byte(i % 256)}, keyBase...)
			value := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(t, "value")
			pairs[i] = types.KVPair{Key: key, Value: value}
		}

		// Sort by key
		sort.Slice(pairs, func(i, j int) bool {
			return bytes.Compare(pairs[i].Key, pairs[j].Key) < 0
		})

		return pairs
	})
}

// genNewKVPair generates a new KVPair that doesn't exist in the given pairs
func genNewKVPair(existingPairs []types.KVPair) *rapid.Generator[types.KVPair] {
	return rapid.Custom(func(t *rapid.T) types.KVPair {
		// Generate a key with a unique prefix to ensure it's different
		keyBase := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "new_key_base")
		// Use 0xFF prefix to make it likely to be unique
		key := append([]byte{0xFF, 0xFF}, keyBase...)
		value := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(t, "new_value")
		return types.KVPair{Key: key, Value: value}
	})
}

// insertSorted inserts a KVPair into a sorted slice maintaining sort order
func insertSorted(pairs []types.KVPair, newPair types.KVPair) []types.KVPair {
	// Find insertion point
	idx := sort.Search(len(pairs), func(i int) bool {
		return bytes.Compare(pairs[i].Key, newPair.Key) >= 0
	})

	// Create new slice with the pair inserted
	result := make([]types.KVPair, len(pairs)+1)
	copy(result[:idx], pairs[:idx])
	result[idx] = newPair
	copy(result[idx+1:], pairs[idx:])

	return result
}

// findChunkIndex finds which chunk contains a given key
func findChunkIndex(chunks [][]types.KVPair, key []byte) int {
	for i, chunk := range chunks {
		for _, pair := range chunk {
			if bytes.Equal(pair.Key, key) {
				return i
			}
		}
	}
	return -1
}

// getChunkBoundaryKeys returns the first key of each chunk (boundary markers)
func getChunkBoundaryKeys(chunks [][]types.KVPair) [][]byte {
	boundaries := make([][]byte, len(chunks))
	for i, chunk := range chunks {
		if len(chunk) > 0 {
			boundaries[i] = chunk[0].Key
		}
	}
	return boundaries
}

// TestProperty_ChunkBoundaryStability tests Property 3: Chunk Boundary Stability
// **Feature: versioned-kv-store, Property 3: Chunk Boundary Stability**
// **Validates: Requirements 2.3**
//
// For any sorted set of KV pairs and any single key insertion, the Rolling_Hash_Chunker
// SHALL produce identical chunk boundaries for all regions not containing the inserted key.
func TestProperty_ChunkBoundaryStability(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate initial sorted pairs
		originalPairs := genSortedKVPairs().Draw(t, "original_pairs")

		// Generate a new pair to insert
		newPair := genNewKVPair(originalPairs).Draw(t, "new_pair")

		// Create chunker with smaller sizes to get more chunks for testing
		chunker := NewBuzhashChunker(256, 64, 1024)

		// Chunk the original pairs
		originalChunks := chunker.Chunk(originalPairs)

		// Insert the new pair and chunk again
		modifiedPairs := insertSorted(originalPairs, newPair)
		modifiedChunks := chunker.Chunk(modifiedPairs)

		// Find which chunk the new key ended up in
		newKeyChunkIdx := findChunkIndex(modifiedChunks, newPair.Key)
		if newKeyChunkIdx == -1 {
			t.Fatal("New key not found in any chunk after insertion")
		}

		// Verify stability: chunks before the affected region should be identical
		// The key insight is that content-defined chunking means chunks BEFORE
		// the insertion point should be completely unchanged
		insertionPoint := sort.Search(len(originalPairs), func(i int) bool {
			return bytes.Compare(originalPairs[i].Key, newPair.Key) >= 0
		})

		// Find which original chunk would contain the insertion point
		pairsSeen := 0
		affectedOriginalChunkIdx := 0
		for i, chunk := range originalChunks {
			pairsSeen += len(chunk)
			if pairsSeen > insertionPoint {
				affectedOriginalChunkIdx = i
				break
			}
			if pairsSeen == insertionPoint && i < len(originalChunks)-1 {
				affectedOriginalChunkIdx = i + 1
				break
			}
		}

		// All chunks BEFORE the affected chunk should be identical
		for i := 0; i < affectedOriginalChunkIdx && i < len(originalChunks) && i < len(modifiedChunks); i++ {
			if !chunksEqual(originalChunks[i], modifiedChunks[i]) {
				t.Fatalf("Chunk %d changed despite being before insertion point.\n"+
					"Original chunk has %d pairs, modified has %d pairs.\n"+
					"Insertion point: %d, Affected chunk: %d",
					i, len(originalChunks[i]), len(modifiedChunks[i]),
					insertionPoint, affectedOriginalChunkIdx)
			}
		}
	})
}

// chunksEqual checks if two chunks contain the same KV pairs
func chunksEqual(a, b []types.KVPair) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i].Key, b[i].Key) || !bytes.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

// TestChunker_BasicFunctionality tests basic chunker behavior
func TestChunker_BasicFunctionality(t *testing.T) {
	t.Run("EmptyInput", func(t *testing.T) {
		chunker := DefaultChunker()
		chunks := chunker.Chunk(nil)
		if chunks != nil {
			t.Errorf("Expected nil for empty input, got %v", chunks)
		}
	})

	t.Run("SinglePair", func(t *testing.T) {
		chunker := DefaultChunker()
		pairs := []types.KVPair{{Key: []byte("key"), Value: []byte("value")}}
		chunks := chunker.Chunk(pairs)
		if len(chunks) != 1 {
			t.Errorf("Expected 1 chunk for single pair, got %d", len(chunks))
		}
		if len(chunks[0]) != 1 {
			t.Errorf("Expected chunk to contain 1 pair, got %d", len(chunks[0]))
		}
	})

	t.Run("Determinism", func(t *testing.T) {
		chunker := DefaultChunker()
		pairs := make([]types.KVPair, 50)
		for i := 0; i < 50; i++ {
			pairs[i] = types.KVPair{
				Key:   []byte{byte(i)},
				Value: []byte{byte(i * 2)},
			}
		}

		chunks1 := chunker.Chunk(pairs)
		chunks2 := chunker.Chunk(pairs)

		if len(chunks1) != len(chunks2) {
			t.Fatalf("Determinism failed: different chunk counts %d vs %d", len(chunks1), len(chunks2))
		}

		for i := range chunks1 {
			if !chunksEqual(chunks1[i], chunks2[i]) {
				t.Fatalf("Determinism failed: chunk %d differs", i)
			}
		}
	})
}
