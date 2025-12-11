package tree

import (
	"bytes"
	"os"
	"sort"
	"testing"

	"microprolly/pkg/cas"
	"microprolly/pkg/chunker"
	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// testDiffSetup creates a temporary CAS, chunker, builder, and diff engine for testing
type testDiffSetup struct {
	tmpDir  string
	cas     *cas.FileCAS
	chunker chunker.Chunker
	builder *TreeBuilder
	differ  *DiffEngine
}

func newTestDiffSetup(t *rapid.T) *testDiffSetup {
	tmpDir, err := os.MkdirTemp("", "diff-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	fileCAS, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create CAS: %v", err)
	}

	ch := chunker.DefaultChunker()
	builder := NewTreeBuilder(fileCAS, ch)
	differ := NewDiffEngine(fileCAS)

	return &testDiffSetup{
		tmpDir:  tmpDir,
		cas:     fileCAS,
		chunker: ch,
		builder: builder,
		differ:  differ,
	}
}

func (s *testDiffSetup) cleanup() {
	s.cas.Close()
	os.RemoveAll(s.tmpDir)
}

// genSortedUniquePairs generates a sorted slice of unique KV pairs
func genSortedUniquePairs() *rapid.Generator[[]types.KVPair] {
	return rapid.Custom(func(t *rapid.T) []types.KVPair {
		count := rapid.IntRange(0, 50).Draw(t, "pair_count")
		if count == 0 {
			return []types.KVPair{}
		}

		pairMap := make(map[string][]byte)
		for i := 0; i < count; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "key")
			value := rapid.SliceOfN(rapid.Byte(), 0, 50).Draw(t, "value")
			pairMap[string(key)] = value
		}

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

// computeExpectedDiff computes the expected diff between two sorted KV pair lists
// This is the "oracle" implementation for testing
func computeExpectedDiff(pairsA, pairsB []types.KVPair) DiffResult {
	result := DiffResult{
		Added:    []types.KVPair{},
		Modified: []ModifiedPair{},
		Deleted:  [][]byte{},
	}

	// Build maps for easier lookup
	mapA := make(map[string][]byte)
	for _, p := range pairsA {
		mapA[string(p.Key)] = p.Value
	}

	mapB := make(map[string][]byte)
	for _, p := range pairsB {
		mapB[string(p.Key)] = p.Value
	}

	// Find deleted and modified keys
	for _, p := range pairsA {
		keyStr := string(p.Key)
		if valB, exists := mapB[keyStr]; exists {
			if !bytes.Equal(p.Value, valB) {
				result.Modified = append(result.Modified, ModifiedPair{
					Key:      p.Key,
					OldValue: p.Value,
					NewValue: valB,
				})
			}
		} else {
			result.Deleted = append(result.Deleted, p.Key)
		}
	}

	// Find added keys
	for _, p := range pairsB {
		keyStr := string(p.Key)
		if _, exists := mapA[keyStr]; !exists {
			result.Added = append(result.Added, p)
		}
	}

	return result
}

// diffResultsEqual compares two DiffResults for equality
func diffResultsEqual(a, b DiffResult) bool {
	// Compare Added
	if len(a.Added) != len(b.Added) {
		return false
	}
	addedMapA := make(map[string][]byte)
	for _, p := range a.Added {
		addedMapA[string(p.Key)] = p.Value
	}
	for _, p := range b.Added {
		if val, exists := addedMapA[string(p.Key)]; !exists || !bytes.Equal(val, p.Value) {
			return false
		}
	}

	// Compare Modified
	if len(a.Modified) != len(b.Modified) {
		return false
	}
	modMapA := make(map[string]ModifiedPair)
	for _, m := range a.Modified {
		modMapA[string(m.Key)] = m
	}
	for _, m := range b.Modified {
		if mod, exists := modMapA[string(m.Key)]; !exists ||
			!bytes.Equal(mod.OldValue, m.OldValue) ||
			!bytes.Equal(mod.NewValue, m.NewValue) {
			return false
		}
	}

	// Compare Deleted
	if len(a.Deleted) != len(b.Deleted) {
		return false
	}
	deletedSetA := make(map[string]bool)
	for _, k := range a.Deleted {
		deletedSetA[string(k)] = true
	}
	for _, k := range b.Deleted {
		if !deletedSetA[string(k)] {
			return false
		}
	}

	return true
}

// TestProperty_DiffCorrectness tests Property 13: Diff Correctness
// **Feature: versioned-kv-store, Property 13: Diff Correctness**
// **Validates: Requirements 7.1**
//
// For any two commits A and B, Diff(A, B) SHALL return exactly the keys that were
// added, modified, or deleted between A and B.
func TestProperty_DiffCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate two different sets of KV pairs
		pairsA := genSortedUniquePairs().Draw(t, "pairs_a")
		pairsB := genSortedUniquePairs().Draw(t, "pairs_b")

		// Build trees for both sets
		hashA, err := setup.builder.Build(pairsA)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}

		hashB, err := setup.builder.Build(pairsB)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Compute diff using our implementation
		actualDiff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Compute expected diff using oracle
		expectedDiff := computeExpectedDiff(pairsA, pairsB)

		// Verify the diff is correct
		if !diffResultsEqual(expectedDiff, actualDiff) {
			t.Fatalf("Diff mismatch:\nExpected: Added=%d, Modified=%d, Deleted=%d\nActual: Added=%d, Modified=%d, Deleted=%d",
				len(expectedDiff.Added), len(expectedDiff.Modified), len(expectedDiff.Deleted),
				len(actualDiff.Added), len(actualDiff.Modified), len(actualDiff.Deleted))
		}
	})
}

// TestProperty_IdenticalTreesEmptyDiff tests Property 14: Identical Trees Have Empty Diff
// **Feature: versioned-kv-store, Property 14: Identical Trees Have Empty Diff**
// **Validates: Requirements 7.2**
//
// For any two commits with identical root hashes, Diff(A, B) SHALL return an empty diff result.
func TestProperty_IdenticalTreesEmptyDiff(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate a set of KV pairs
		pairs := genSortedUniquePairs().Draw(t, "pairs")

		// Build tree
		hash, err := setup.builder.Build(pairs)
		if err != nil {
			t.Fatalf("Failed to build tree: %v", err)
		}

		// Diff the tree with itself
		diff, err := setup.differ.Diff(hash, hash)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Verify the diff is empty
		if len(diff.Added) != 0 {
			t.Fatalf("Expected no added keys, got %d", len(diff.Added))
		}
		if len(diff.Modified) != 0 {
			t.Fatalf("Expected no modified keys, got %d", len(diff.Modified))
		}
		if len(diff.Deleted) != 0 {
			t.Fatalf("Expected no deleted keys, got %d", len(diff.Deleted))
		}
	})
}

// --- Additional extensive tests for incremental changes ---

// newTestDiffSetupStd creates a test setup for standard testing.T
func newTestDiffSetupStd(t *testing.T) *testDiffSetup {
	tmpDir, err := os.MkdirTemp("", "diff-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	fileCAS, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create CAS: %v", err)
	}

	ch := chunker.DefaultChunker()
	builder := NewTreeBuilder(fileCAS, ch)
	differ := NewDiffEngine(fileCAS)

	return &testDiffSetup{
		tmpDir:  tmpDir,
		cas:     fileCAS,
		chunker: ch,
		builder: builder,
		differ:  differ,
	}
}

// TestProperty_SingleKeyAddition tests diff when adding a single key to an existing tree
// This is the critical "small change" scenario the diff algorithm optimizes for
func TestProperty_SingleKeyAddition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate base pairs (at least 10 to ensure multi-level tree)
		basePairs := rapid.Custom(func(t *rapid.T) []types.KVPair {
			count := rapid.IntRange(10, 100).Draw(t, "base_count")
			pairMap := make(map[string][]byte)
			for i := 0; i < count; i++ {
				key := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "key")
				value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "value")
				pairMap[string(key)] = value
			}
			pairs := make([]types.KVPair, 0, len(pairMap))
			for k, v := range pairMap {
				pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
			}
			sort.Slice(pairs, func(i, j int) bool {
				return string(pairs[i].Key) < string(pairs[j].Key)
			})
			return pairs
		}).Draw(t, "base_pairs")

		// Generate a new key that doesn't exist in base
		var newKey []byte
		for {
			newKey = rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "new_key")
			exists := false
			for _, p := range basePairs {
				if bytes.Equal(p.Key, newKey) {
					exists = true
					break
				}
			}
			if !exists {
				break
			}
		}
		newValue := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "new_value")

		// Create modified pairs with the new key
		modifiedPairs := make([]types.KVPair, len(basePairs)+1)
		copy(modifiedPairs, basePairs)
		modifiedPairs[len(basePairs)] = types.KVPair{Key: newKey, Value: newValue}
		sort.Slice(modifiedPairs, func(i, j int) bool {
			return string(modifiedPairs[i].Key) < string(modifiedPairs[j].Key)
		})

		// Build both trees
		hashA, err := setup.builder.Build(basePairs)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(modifiedPairs)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Compute diff
		diff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Verify: exactly one addition, no modifications, no deletions
		if len(diff.Added) != 1 {
			t.Fatalf("Expected 1 added key, got %d", len(diff.Added))
		}
		if !bytes.Equal(diff.Added[0].Key, newKey) || !bytes.Equal(diff.Added[0].Value, newValue) {
			t.Fatalf("Added key mismatch")
		}
		if len(diff.Modified) != 0 {
			t.Fatalf("Expected 0 modified keys, got %d", len(diff.Modified))
		}
		if len(diff.Deleted) != 0 {
			t.Fatalf("Expected 0 deleted keys, got %d", len(diff.Deleted))
		}
	})
}

// TestProperty_SingleKeyDeletion tests diff when deleting a single key
func TestProperty_SingleKeyDeletion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate base pairs (at least 10)
		basePairs := rapid.Custom(func(t *rapid.T) []types.KVPair {
			count := rapid.IntRange(10, 100).Draw(t, "base_count")
			pairMap := make(map[string][]byte)
			for i := 0; i < count; i++ {
				key := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "key")
				value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "value")
				pairMap[string(key)] = value
			}
			pairs := make([]types.KVPair, 0, len(pairMap))
			for k, v := range pairMap {
				pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
			}
			sort.Slice(pairs, func(i, j int) bool {
				return string(pairs[i].Key) < string(pairs[j].Key)
			})
			return pairs
		}).Draw(t, "base_pairs")

		if len(basePairs) == 0 {
			return // Skip empty case
		}

		// Pick a random key to delete
		deleteIdx := rapid.IntRange(0, len(basePairs)-1).Draw(t, "delete_idx")
		deletedKey := basePairs[deleteIdx].Key

		// Create modified pairs without the deleted key
		modifiedPairs := make([]types.KVPair, 0, len(basePairs)-1)
		for i, p := range basePairs {
			if i != deleteIdx {
				modifiedPairs = append(modifiedPairs, p)
			}
		}

		// Build both trees
		hashA, err := setup.builder.Build(basePairs)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(modifiedPairs)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Compute diff
		diff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Verify: exactly one deletion, no additions, no modifications
		if len(diff.Deleted) != 1 {
			t.Fatalf("Expected 1 deleted key, got %d", len(diff.Deleted))
		}
		if !bytes.Equal(diff.Deleted[0], deletedKey) {
			t.Fatalf("Deleted key mismatch")
		}
		if len(diff.Added) != 0 {
			t.Fatalf("Expected 0 added keys, got %d", len(diff.Added))
		}
		if len(diff.Modified) != 0 {
			t.Fatalf("Expected 0 modified keys, got %d", len(diff.Modified))
		}
	})
}

// TestProperty_SingleKeyModification tests diff when modifying a single key's value
func TestProperty_SingleKeyModification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate base pairs (at least 10)
		basePairs := rapid.Custom(func(t *rapid.T) []types.KVPair {
			count := rapid.IntRange(10, 100).Draw(t, "base_count")
			pairMap := make(map[string][]byte)
			for i := 0; i < count; i++ {
				key := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "key")
				value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "value")
				pairMap[string(key)] = value
			}
			pairs := make([]types.KVPair, 0, len(pairMap))
			for k, v := range pairMap {
				pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
			}
			sort.Slice(pairs, func(i, j int) bool {
				return string(pairs[i].Key) < string(pairs[j].Key)
			})
			return pairs
		}).Draw(t, "base_pairs")

		if len(basePairs) == 0 {
			return // Skip empty case
		}

		// Pick a random key to modify
		modifyIdx := rapid.IntRange(0, len(basePairs)-1).Draw(t, "modify_idx")
		modifiedKey := basePairs[modifyIdx].Key
		oldValue := basePairs[modifyIdx].Value

		// Generate a new value different from the old one
		var newValue []byte
		for {
			newValue = rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "new_value")
			if !bytes.Equal(newValue, oldValue) {
				break
			}
		}

		// Create modified pairs
		modifiedPairs := make([]types.KVPair, len(basePairs))
		copy(modifiedPairs, basePairs)
		modifiedPairs[modifyIdx] = types.KVPair{Key: modifiedKey, Value: newValue}

		// Build both trees
		hashA, err := setup.builder.Build(basePairs)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(modifiedPairs)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Compute diff
		diff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Verify: exactly one modification, no additions, no deletions
		if len(diff.Modified) != 1 {
			t.Fatalf("Expected 1 modified key, got %d", len(diff.Modified))
		}
		if !bytes.Equal(diff.Modified[0].Key, modifiedKey) {
			t.Fatalf("Modified key mismatch")
		}
		if !bytes.Equal(diff.Modified[0].OldValue, oldValue) {
			t.Fatalf("Old value mismatch")
		}
		if !bytes.Equal(diff.Modified[0].NewValue, newValue) {
			t.Fatalf("New value mismatch")
		}
		if len(diff.Added) != 0 {
			t.Fatalf("Expected 0 added keys, got %d", len(diff.Added))
		}
		if len(diff.Deleted) != 0 {
			t.Fatalf("Expected 0 deleted keys, got %d", len(diff.Deleted))
		}
	})
}

// TestProperty_MultipleSmallChanges tests diff with a few additions, deletions, and modifications
func TestProperty_MultipleSmallChanges(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		// Generate base pairs (at least 20)
		basePairs := rapid.Custom(func(t *rapid.T) []types.KVPair {
			count := rapid.IntRange(20, 100).Draw(t, "base_count")
			pairMap := make(map[string][]byte)
			for i := 0; i < count; i++ {
				key := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "key")
				value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "value")
				pairMap[string(key)] = value
			}
			pairs := make([]types.KVPair, 0, len(pairMap))
			for k, v := range pairMap {
				pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
			}
			sort.Slice(pairs, func(i, j int) bool {
				return string(pairs[i].Key) < string(pairs[j].Key)
			})
			return pairs
		}).Draw(t, "base_pairs")

		if len(basePairs) < 5 {
			return // Need enough keys to work with
		}

		// Track expected changes
		expectedAdded := []types.KVPair{}
		expectedModified := []ModifiedPair{}
		expectedDeleted := [][]byte{}

		// Create a map for modifications
		pairMap := make(map[string][]byte)
		for _, p := range basePairs {
			pairMap[string(p.Key)] = p.Value
		}

		// Delete 1-3 random keys
		numDeletes := rapid.IntRange(1, 3).Draw(t, "num_deletes")
		deletedIndices := make(map[int]bool)
		for i := 0; i < numDeletes && i < len(basePairs); i++ {
			idx := rapid.IntRange(0, len(basePairs)-1).Draw(t, "delete_idx")
			if !deletedIndices[idx] {
				deletedIndices[idx] = true
				expectedDeleted = append(expectedDeleted, basePairs[idx].Key)
				delete(pairMap, string(basePairs[idx].Key))
			}
		}

		// Modify 1-3 random keys (that weren't deleted)
		numModifies := rapid.IntRange(1, 3).Draw(t, "num_modifies")
		modifiedIndices := make(map[int]bool)
		for i := 0; i < numModifies; i++ {
			idx := rapid.IntRange(0, len(basePairs)-1).Draw(t, "modify_idx")
			if !deletedIndices[idx] && !modifiedIndices[idx] {
				modifiedIndices[idx] = true
				oldValue := basePairs[idx].Value
				newValue := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "new_value")
				if !bytes.Equal(newValue, oldValue) {
					expectedModified = append(expectedModified, ModifiedPair{
						Key:      basePairs[idx].Key,
						OldValue: oldValue,
						NewValue: newValue,
					})
					pairMap[string(basePairs[idx].Key)] = newValue
				}
			}
		}

		// Add 1-3 new keys
		numAdds := rapid.IntRange(1, 3).Draw(t, "num_adds")
		for i := 0; i < numAdds; i++ {
			newKey := rapid.SliceOfN(rapid.Byte(), 1, 30).Draw(t, "new_key")
			if _, exists := pairMap[string(newKey)]; !exists {
				newValue := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(t, "new_value")
				expectedAdded = append(expectedAdded, types.KVPair{Key: newKey, Value: newValue})
				pairMap[string(newKey)] = newValue
			}
		}

		// Build modified pairs from map
		modifiedPairs := make([]types.KVPair, 0, len(pairMap))
		for k, v := range pairMap {
			modifiedPairs = append(modifiedPairs, types.KVPair{Key: []byte(k), Value: v})
		}
		sort.Slice(modifiedPairs, func(i, j int) bool {
			return string(modifiedPairs[i].Key) < string(modifiedPairs[j].Key)
		})

		// Build both trees
		hashA, err := setup.builder.Build(basePairs)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(modifiedPairs)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Compute diff
		diff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Verify counts match
		if len(diff.Added) != len(expectedAdded) {
			t.Fatalf("Added count mismatch: expected %d, got %d", len(expectedAdded), len(diff.Added))
		}
		if len(diff.Modified) != len(expectedModified) {
			t.Fatalf("Modified count mismatch: expected %d, got %d", len(expectedModified), len(diff.Modified))
		}
		if len(diff.Deleted) != len(expectedDeleted) {
			t.Fatalf("Deleted count mismatch: expected %d, got %d", len(expectedDeleted), len(diff.Deleted))
		}

		// Verify using oracle
		oracleDiff := computeExpectedDiff(basePairs, modifiedPairs)
		if !diffResultsEqual(oracleDiff, diff) {
			t.Fatalf("Diff doesn't match oracle")
		}
	})
}

// TestProperty_DiffSymmetry tests that Diff(A,B) and Diff(B,A) are inverses
func TestProperty_DiffSymmetry(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		pairsA := genSortedUniquePairs().Draw(t, "pairs_a")
		pairsB := genSortedUniquePairs().Draw(t, "pairs_b")

		hashA, err := setup.builder.Build(pairsA)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(pairsB)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		// Diff A->B
		diffAB, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff A->B failed: %v", err)
		}

		// Diff B->A
		diffBA, err := setup.differ.Diff(hashB, hashA)
		if err != nil {
			t.Fatalf("Diff B->A failed: %v", err)
		}

		// Added in A->B should be Deleted in B->A
		if len(diffAB.Added) != len(diffBA.Deleted) {
			t.Fatalf("Symmetry failed: Added(A->B)=%d, Deleted(B->A)=%d",
				len(diffAB.Added), len(diffBA.Deleted))
		}

		// Deleted in A->B should be Added in B->A
		if len(diffAB.Deleted) != len(diffBA.Added) {
			t.Fatalf("Symmetry failed: Deleted(A->B)=%d, Added(B->A)=%d",
				len(diffAB.Deleted), len(diffBA.Added))
		}

		// Modified count should be the same
		if len(diffAB.Modified) != len(diffBA.Modified) {
			t.Fatalf("Symmetry failed: Modified counts differ")
		}

		// Verify added keys in A->B match deleted keys in B->A
		addedKeysAB := make(map[string]bool)
		for _, p := range diffAB.Added {
			addedKeysAB[string(p.Key)] = true
		}
		for _, k := range diffBA.Deleted {
			if !addedKeysAB[string(k)] {
				t.Fatalf("Symmetry failed: key in Deleted(B->A) not in Added(A->B)")
			}
		}
	})
}

// TestProperty_DiffApplyReconstruction tests that applying diff to A produces B
func TestProperty_DiffApplyReconstruction(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup := newTestDiffSetup(t)
		defer setup.cleanup()

		pairsA := genSortedUniquePairs().Draw(t, "pairs_a")
		pairsB := genSortedUniquePairs().Draw(t, "pairs_b")

		hashA, err := setup.builder.Build(pairsA)
		if err != nil {
			t.Fatalf("Failed to build tree A: %v", err)
		}
		hashB, err := setup.builder.Build(pairsB)
		if err != nil {
			t.Fatalf("Failed to build tree B: %v", err)
		}

		diff, err := setup.differ.Diff(hashA, hashB)
		if err != nil {
			t.Fatalf("Diff failed: %v", err)
		}

		// Apply diff to A to reconstruct B
		reconstructed := make(map[string][]byte)
		for _, p := range pairsA {
			reconstructed[string(p.Key)] = p.Value
		}

		// Apply deletions
		for _, k := range diff.Deleted {
			delete(reconstructed, string(k))
		}

		// Apply modifications
		for _, m := range diff.Modified {
			reconstructed[string(m.Key)] = m.NewValue
		}

		// Apply additions
		for _, p := range diff.Added {
			reconstructed[string(p.Key)] = p.Value
		}

		// Convert B to map for comparison
		expectedB := make(map[string][]byte)
		for _, p := range pairsB {
			expectedB[string(p.Key)] = p.Value
		}

		// Verify reconstruction matches B
		if len(reconstructed) != len(expectedB) {
			t.Fatalf("Reconstruction size mismatch: got %d, expected %d",
				len(reconstructed), len(expectedB))
		}

		for k, v := range expectedB {
			if rv, exists := reconstructed[k]; !exists {
				t.Fatalf("Key %q missing from reconstruction", k)
			} else if !bytes.Equal(rv, v) {
				t.Fatalf("Value mismatch for key %q", k)
			}
		}
	})
}

// TestDiff_EmptyTrees tests diffing empty trees
func TestDiff_EmptyTrees(t *testing.T) {
	setup := newTestDiffSetupStd(t)
	defer setup.cleanup()

	// Build two empty trees
	hashA, err := setup.builder.Build([]types.KVPair{})
	if err != nil {
		t.Fatalf("Failed to build empty tree A: %v", err)
	}
	hashB, err := setup.builder.Build([]types.KVPair{})
	if err != nil {
		t.Fatalf("Failed to build empty tree B: %v", err)
	}

	diff, err := setup.differ.Diff(hashA, hashB)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 0 || len(diff.Modified) != 0 || len(diff.Deleted) != 0 {
		t.Fatalf("Expected empty diff for two empty trees")
	}
}

// TestDiff_EmptyToNonEmpty tests diffing empty tree to non-empty
func TestDiff_EmptyToNonEmpty(t *testing.T) {
	setup := newTestDiffSetupStd(t)
	defer setup.cleanup()

	pairs := []types.KVPair{
		{Key: []byte("a"), Value: []byte("1")},
		{Key: []byte("b"), Value: []byte("2")},
		{Key: []byte("c"), Value: []byte("3")},
	}

	hashEmpty, _ := setup.builder.Build([]types.KVPair{})
	hashFull, _ := setup.builder.Build(pairs)

	diff, err := setup.differ.Diff(hashEmpty, hashFull)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 3 {
		t.Fatalf("Expected 3 additions, got %d", len(diff.Added))
	}
	if len(diff.Modified) != 0 || len(diff.Deleted) != 0 {
		t.Fatalf("Expected no modifications or deletions")
	}
}

// TestDiff_NonEmptyToEmpty tests diffing non-empty tree to empty
func TestDiff_NonEmptyToEmpty(t *testing.T) {
	setup := newTestDiffSetupStd(t)
	defer setup.cleanup()

	pairs := []types.KVPair{
		{Key: []byte("a"), Value: []byte("1")},
		{Key: []byte("b"), Value: []byte("2")},
		{Key: []byte("c"), Value: []byte("3")},
	}

	hashFull, _ := setup.builder.Build(pairs)
	hashEmpty, _ := setup.builder.Build([]types.KVPair{})

	diff, err := setup.differ.Diff(hashFull, hashEmpty)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Deleted) != 3 {
		t.Fatalf("Expected 3 deletions, got %d", len(diff.Deleted))
	}
	if len(diff.Modified) != 0 || len(diff.Added) != 0 {
		t.Fatalf("Expected no modifications or additions")
	}
}
