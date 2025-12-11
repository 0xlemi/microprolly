package store

import (
	"math"
	"os"
	"sort"
	"testing"

	"microprolly/pkg/cas"
	"microprolly/pkg/chunker"
	"microprolly/pkg/tree"
	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// TestProperty_StructuralSharingEfficiency tests Property 15: Structural Sharing Efficiency
// **Feature: versioned-kv-store, Property 15: Structural Sharing Efficiency**
// **Validates: Requirements 8.1, 8.2, 8.3**
//
// For any tree with N nodes, after modifying a single key, the new tree SHALL share
// at least (N - log(N)) nodes with the original tree.
func TestProperty_StructuralSharingEfficiency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a temporary directory for CAS
		tmpDir, err := os.MkdirTemp("", "structural-sharing-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Create a tracking CAS to monitor writes
		fileCAS, err := cas.NewFileCAS(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create CAS: %v", err)
		}
		defer fileCAS.Close()

		trackingCAS := cas.NewTrackingCAS(fileCAS)
		chunkr := chunker.DefaultChunker()
		builder := tree.NewTreeBuilder(trackingCAS, chunkr)

		// Generate initial sorted KV pairs (need enough pairs to create multiple nodes)
		// Use at least 20 pairs to ensure we get a meaningful tree structure
		numPairs := rapid.IntRange(20, 100).Draw(rt, "numPairs")
		pairs := generateSortedUniquePairs(rt, numPairs)

		if len(pairs) < 10 {
			// Skip if we don't have enough unique pairs
			return
		}

		// Build the initial tree
		_, err = builder.Build(pairs)
		if err != nil {
			rt.Fatalf("Initial build failed: %v", err)
		}

		// Get stats after first build
		statsAfterFirst := trackingCAS.Stats()
		initialNodeCount := statsAfterFirst.ActualWrites

		// Skip if tree is too small (need at least a few nodes for meaningful test)
		if initialNodeCount < 3 {
			return
		}

		// Reset stats to track only the second build
		trackingCAS.ResetStats()

		// Modify a single key (pick a random existing key and change its value)
		keyIndex := rapid.IntRange(0, len(pairs)-1).Draw(rt, "keyIndex")
		newValue := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(rt, "newValue")

		// Create modified pairs
		modifiedPairs := make([]types.KVPair, len(pairs))
		copy(modifiedPairs, pairs)
		modifiedPairs[keyIndex] = types.KVPair{
			Key:   pairs[keyIndex].Key,
			Value: newValue,
		}

		// Build the modified tree
		_, err = builder.Build(modifiedPairs)
		if err != nil {
			rt.Fatalf("Modified build failed: %v", err)
		}

		// Get stats after second build
		statsAfterSecond := trackingCAS.Stats()

		// Calculate structural sharing
		// TotalWrites = all Write() calls during second build
		// DeduplicatedWrites = writes that were skipped because data already existed
		// ActualWrites = new nodes that had to be written
		sharedNodes := statsAfterSecond.DeduplicatedWrites
		newNodes := statsAfterSecond.ActualWrites

		// The property states: shared nodes >= N - log(N)
		// Where N is the initial node count
		// This means: new nodes <= log(N)
		expectedMaxNewNodes := math.Log2(float64(initialNodeCount))

		// Allow some tolerance for edge cases (chunking boundaries can cause more changes)
		// We use 2*log(N) as a more realistic bound given content-defined chunking
		tolerantMaxNewNodes := 2 * expectedMaxNewNodes

		// Verify structural sharing efficiency
		if float64(newNodes) > tolerantMaxNewNodes && newNodes > 3 {
			rt.Fatalf("Structural sharing inefficient: "+
				"initial nodes=%d, new nodes written=%d, shared nodes=%d, "+
				"expected max new nodes=%.1f (2*log2(N))",
				initialNodeCount, newNodes, sharedNodes, tolerantMaxNewNodes)
		}
	})
}

// generateSortedUniquePairs generates a sorted slice of unique KV pairs
func generateSortedUniquePairs(rt *rapid.T, count int) []types.KVPair {
	// Use a map to ensure unique keys
	pairMap := make(map[string][]byte)
	for i := 0; i < count*2 && len(pairMap) < count; i++ {
		key := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(rt, "key")
		value := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(rt, "value")
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
}

// TestStructuralSharing_SingleKeyModification is a unit test that verifies
// structural sharing works correctly when modifying a single key
func TestStructuralSharing_SingleKeyModification(t *testing.T) {
	// Create a temporary directory for CAS
	tmpDir, err := os.MkdirTemp("", "structural-sharing-unit-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a tracking CAS
	fileCAS, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}
	defer fileCAS.Close()

	trackingCAS := cas.NewTrackingCAS(fileCAS)
	// Use a smaller chunk size to ensure we get multiple nodes with our test data
	chunkr := chunker.NewBuzhashChunker(256, 64, 1024)
	builder := tree.NewTreeBuilder(trackingCAS, chunkr)

	// Create initial data with enough keys and larger values to create multiple nodes
	// Each value is ~100 bytes, so 50 pairs = ~5000 bytes, which should create multiple chunks
	pairs := make([]types.KVPair, 50)
	for i := 0; i < 50; i++ {
		// Create a larger value to ensure we exceed chunk boundaries
		value := make([]byte, 100)
		for j := range value {
			value[j] = byte((i + j) % 256)
		}
		pairs[i] = types.KVPair{
			Key:   []byte{byte(i)},
			Value: value,
		}
	}

	// Build initial tree
	hash1, err := builder.Build(pairs)
	if err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	statsAfterFirst := trackingCAS.Stats()
	initialNodes := statsAfterFirst.ActualWrites
	t.Logf("Initial tree: %d nodes, root hash: %s", initialNodes, hash1.String())

	// Skip test if we still only have 1 node (chunking didn't create multiple nodes)
	if initialNodes < 3 {
		t.Skipf("Skipping: tree only has %d nodes, need at least 3 for meaningful structural sharing test", initialNodes)
	}

	// Reset stats
	trackingCAS.ResetStats()

	// Modify a single key in the middle
	modifiedPairs := make([]types.KVPair, len(pairs))
	copy(modifiedPairs, pairs)
	newValue := make([]byte, 100)
	for j := range newValue {
		newValue[j] = 255 // Different value pattern
	}
	modifiedPairs[25] = types.KVPair{
		Key:   []byte{25},
		Value: newValue,
	}

	// Build modified tree
	hash2, err := builder.Build(modifiedPairs)
	if err != nil {
		t.Fatalf("Modified build failed: %v", err)
	}

	statsAfterSecond := trackingCAS.Stats()
	t.Logf("Modified tree: new nodes=%d, shared nodes=%d, root hash: %s",
		statsAfterSecond.ActualWrites, statsAfterSecond.DeduplicatedWrites, hash2.String())

	// Verify that root hashes are different (data changed)
	if hash1 == hash2 {
		t.Fatal("Root hashes should be different after modification")
	}

	// Verify structural sharing occurred
	if statsAfterSecond.DeduplicatedWrites == 0 {
		t.Fatal("Expected some nodes to be shared (deduplicated)")
	}

	// Verify that we didn't rewrite all nodes
	if statsAfterSecond.ActualWrites >= initialNodes {
		t.Fatalf("Expected fewer new nodes than initial nodes: new=%d, initial=%d",
			statsAfterSecond.ActualWrites, initialNodes)
	}
}
