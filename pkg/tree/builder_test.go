package tree

import (
	"os"
	"sort"
	"testing"

	"microprolly/pkg/cas"
	"microprolly/pkg/chunker"
	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// genSortedKVPairs generates a sorted slice of unique KV pairs
func genSortedKVPairs() *rapid.Generator[[]types.KVPair] {
	return rapid.Custom(func(t *rapid.T) []types.KVPair {
		// Generate between 0 and 100 pairs
		count := rapid.IntRange(0, 100).Draw(t, "pair_count")
		if count == 0 {
			return []types.KVPair{}
		}

		// Use a map to ensure unique keys
		pairMap := make(map[string][]byte)
		for i := 0; i < count; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "key")
			value := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "value")
			pairMap[string(key)] = value
		}

		// Convert to slice and sort by key
		pairs := make([]types.KVPair, 0, len(pairMap))
		for k, v := range pairMap {
			pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
		}

		sort.Slice(pairs, func(i, j int) bool {
			return string(pairs[i].Key) < string(pairs[j].Key)
		})

		return pairs
	})
}

// TestProperty_TreeConstructionDeterminism tests Property 6: Tree Construction Determinism
// **Feature: versioned-kv-store, Property 6: Tree Construction Determinism**
// **Validates: Requirements 3.4**
//
// For any sorted set of KV pairs, building a Prolly Tree multiple times SHALL produce identical root hashes.
func TestProperty_TreeConstructionDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pairs := genSortedKVPairs().Draw(t, "sorted_pairs")

		// Create two separate CAS instances
		tmpDir1, err := os.MkdirTemp("", "tree-test-1-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir 1: %v", err)
		}
		defer os.RemoveAll(tmpDir1)

		tmpDir2, err := os.MkdirTemp("", "tree-test-2-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir 2: %v", err)
		}
		defer os.RemoveAll(tmpDir2)

		cas1, err := cas.NewFileCAS(tmpDir1)
		if err != nil {
			t.Fatalf("Failed to create CAS 1: %v", err)
		}
		defer cas1.Close()

		cas2, err := cas.NewFileCAS(tmpDir2)
		if err != nil {
			t.Fatalf("Failed to create CAS 2: %v", err)
		}
		defer cas2.Close()

		// Use identical chunkers
		chunker1 := chunker.DefaultChunker()
		chunker2 := chunker.DefaultChunker()

		// Build trees with identical data
		builder1 := NewTreeBuilder(cas1, chunker1)
		builder2 := NewTreeBuilder(cas2, chunker2)

		hash1, err := builder1.Build(pairs)
		if err != nil {
			t.Fatalf("First build failed: %v", err)
		}

		hash2, err := builder2.Build(pairs)
		if err != nil {
			t.Fatalf("Second build failed: %v", err)
		}

		// Verify determinism: both builds should produce identical root hashes
		if hash1 != hash2 {
			t.Fatalf("Determinism failed: root hashes differ\nFirst:  %s\nSecond: %s", hash1.String(), hash2.String())
		}
	})
}
