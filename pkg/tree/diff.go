package tree

import (
	"bytes"

	"microprolly/pkg/cas"
	"microprolly/pkg/types"
)

// DiffResult contains the differences between two tree versions
type DiffResult struct {
	Added    []types.KVPair // Keys present in B but not A
	Modified []ModifiedPair // Keys with different values
	Deleted  [][]byte       // Keys present in A but not B
}

// ModifiedPair represents a key that was modified between versions
type ModifiedPair struct {
	Key      []byte
	OldValue []byte
	NewValue []byte
}

// DiffEngine computes differences between tree versions
type DiffEngine struct {
	cas cas.CAS
}

// NewDiffEngine creates a new DiffEngine with the given CAS
func NewDiffEngine(cas cas.CAS) *DiffEngine {
	return &DiffEngine{cas: cas}
}

// Diff returns changes between two tree roots.
// Requirements 7.1, 7.2, 7.3, 7.4:
// - Returns added, modified, and deleted keys
// - Early exit if root hashes are identical
// - Skips subtrees with matching hashes
// - Recursively compares only differing subtrees
func (d *DiffEngine) Diff(hashA, hashB types.Hash) (DiffResult, error) {
	result := DiffResult{
		Added:    []types.KVPair{},
		Modified: []ModifiedPair{},
		Deleted:  [][]byte{},
	}

	// Requirement 7.2: Early exit if identical root hashes
	if hashA == hashB {
		return result, nil
	}

	// Load both root nodes
	nodeA, err := d.loadNode(hashA)
	if err != nil {
		return result, err
	}

	nodeB, err := d.loadNode(hashB)
	if err != nil {
		return result, err
	}

	// Recursively diff the trees
	err = d.diffNodes(nodeA, nodeB, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

// loadNode loads a node from CAS by its hash
func (d *DiffEngine) loadNode(hash types.Hash) (types.Node, error) {
	data, err := d.cas.Read(hash)
	if err != nil {
		return nil, err
	}
	return DeserializeNode(data)
}

// diffNodes recursively compares two nodes and collects differences
func (d *DiffEngine) diffNodes(nodeA, nodeB types.Node, result *DiffResult) error {
	// Both are leaf nodes - compare KV pairs directly
	if nodeA.IsLeaf() && nodeB.IsLeaf() {
		leafA := nodeA.(*types.LeafNode)
		leafB := nodeB.(*types.LeafNode)
		d.diffLeaves(leafA, leafB, result)
		return nil
	}

	// Both are internal nodes - compare children
	if !nodeA.IsLeaf() && !nodeB.IsLeaf() {
		internalA := nodeA.(*types.InternalNode)
		internalB := nodeB.(*types.InternalNode)
		return d.diffInternalNodes(internalA, internalB, result)
	}

	// Mixed node types - collect all from both and diff
	// This can happen when tree structure changes significantly
	pairsA, err := d.collectAllPairs(nodeA)
	if err != nil {
		return err
	}
	pairsB, err := d.collectAllPairs(nodeB)
	if err != nil {
		return err
	}
	d.diffPairLists(pairsA, pairsB, result)
	return nil
}

// diffLeaves compares two leaf nodes and collects differences
func (d *DiffEngine) diffLeaves(leafA, leafB *types.LeafNode, result *DiffResult) {
	d.diffPairLists(leafA.Pairs, leafB.Pairs, result)
}

// diffPairLists compares two sorted lists of KV pairs
func (d *DiffEngine) diffPairLists(pairsA, pairsB []types.KVPair, result *DiffResult) {
	i, j := 0, 0

	for i < len(pairsA) && j < len(pairsB) {
		cmp := bytes.Compare(pairsA[i].Key, pairsB[j].Key)

		if cmp < 0 {
			// Key exists in A but not B - deleted
			keyCopy := make([]byte, len(pairsA[i].Key))
			copy(keyCopy, pairsA[i].Key)
			result.Deleted = append(result.Deleted, keyCopy)
			i++
		} else if cmp > 0 {
			// Key exists in B but not A - added
			result.Added = append(result.Added, copyKVPair(pairsB[j]))
			j++
		} else {
			// Same key - check if value changed
			if !bytes.Equal(pairsA[i].Value, pairsB[j].Value) {
				result.Modified = append(result.Modified, ModifiedPair{
					Key:      copyBytes(pairsA[i].Key),
					OldValue: copyBytes(pairsA[i].Value),
					NewValue: copyBytes(pairsB[j].Value),
				})
			}
			i++
			j++
		}
	}

	// Remaining in A are deleted
	for ; i < len(pairsA); i++ {
		keyCopy := make([]byte, len(pairsA[i].Key))
		copy(keyCopy, pairsA[i].Key)
		result.Deleted = append(result.Deleted, keyCopy)
	}

	// Remaining in B are added
	for ; j < len(pairsB); j++ {
		result.Added = append(result.Added, copyKVPair(pairsB[j]))
	}
}

// diffInternalNodes compares two internal nodes
// Requirement 7.3, 7.4: Skip matching subtrees, recursively compare differing ones
func (d *DiffEngine) diffInternalNodes(nodeA, nodeB *types.InternalNode, result *DiffResult) error {
	childrenA := nodeA.Children
	childrenB := nodeB.Children

	i, j := 0, 0

	for i < len(childrenA) && j < len(childrenB) {
		// Determine the key ranges for comparison
		keyA := childrenA[i].Key
		keyB := childrenB[j].Key

		// Get the upper bound keys (next child's key or nil if last)
		var upperA, upperB []byte
		if i+1 < len(childrenA) {
			upperA = childrenA[i+1].Key
		}
		if j+1 < len(childrenB) {
			upperB = childrenB[j+1].Key
		}

		// Check if these children cover overlapping key ranges
		cmpKeys := bytes.Compare(keyA, keyB)

		if cmpKeys == 0 {
			// Same starting key - compare these children
			if childrenA[i].Hash == childrenB[j].Hash {
				// Requirement 7.3: Skip subtrees with matching hashes
				i++
				j++
				continue
			}

			// Hashes differ - recursively compare (Requirement 7.4)
			childNodeA, err := d.loadNode(childrenA[i].Hash)
			if err != nil {
				return err
			}
			childNodeB, err := d.loadNode(childrenB[j].Hash)
			if err != nil {
				return err
			}
			if err := d.diffNodes(childNodeA, childNodeB, result); err != nil {
				return err
			}
			i++
			j++
		} else if cmpKeys < 0 {
			// Child A starts before child B
			// Check if A's range overlaps with B
			if upperA == nil || bytes.Compare(upperA, keyB) > 0 {
				// Ranges overlap - need to compare at pair level
				pairsA, err := d.collectPairsInRange(childrenA[i].Hash, keyA, upperA)
				if err != nil {
					return err
				}
				pairsB, err := d.collectPairsInRange(childrenB[j].Hash, keyB, upperB)
				if err != nil {
					return err
				}

				// Filter pairs from A that are before B's range
				var beforeB []types.KVPair
				var overlapA []types.KVPair
				for _, p := range pairsA {
					if bytes.Compare(p.Key, keyB) < 0 {
						beforeB = append(beforeB, p)
					} else {
						overlapA = append(overlapA, p)
					}
				}

				// Keys before B's range are deleted
				for _, p := range beforeB {
					result.Deleted = append(result.Deleted, copyBytes(p.Key))
				}

				// Compare overlapping portions
				d.diffPairLists(overlapA, pairsB, result)
				i++
				j++
			} else {
				// A's range is entirely before B - all keys in A are deleted
				pairs, err := d.collectAllPairsFromHash(childrenA[i].Hash)
				if err != nil {
					return err
				}
				for _, p := range pairs {
					result.Deleted = append(result.Deleted, copyBytes(p.Key))
				}
				i++
			}
		} else {
			// Child B starts before child A
			// Check if B's range overlaps with A
			if upperB == nil || bytes.Compare(upperB, keyA) > 0 {
				// Ranges overlap - need to compare at pair level
				pairsA, err := d.collectPairsInRange(childrenA[i].Hash, keyA, upperA)
				if err != nil {
					return err
				}
				pairsB, err := d.collectPairsInRange(childrenB[j].Hash, keyB, upperB)
				if err != nil {
					return err
				}

				// Filter pairs from B that are before A's range
				var beforeA []types.KVPair
				var overlapB []types.KVPair
				for _, p := range pairsB {
					if bytes.Compare(p.Key, keyA) < 0 {
						beforeA = append(beforeA, p)
					} else {
						overlapB = append(overlapB, p)
					}
				}

				// Keys before A's range are added
				for _, p := range beforeA {
					result.Added = append(result.Added, copyKVPair(p))
				}

				// Compare overlapping portions
				d.diffPairLists(pairsA, overlapB, result)
				i++
				j++
			} else {
				// B's range is entirely before A - all keys in B are added
				pairs, err := d.collectAllPairsFromHash(childrenB[j].Hash)
				if err != nil {
					return err
				}
				for _, p := range pairs {
					result.Added = append(result.Added, copyKVPair(p))
				}
				j++
			}
		}
	}

	// Remaining children in A are deleted
	for ; i < len(childrenA); i++ {
		pairs, err := d.collectAllPairsFromHash(childrenA[i].Hash)
		if err != nil {
			return err
		}
		for _, p := range pairs {
			result.Deleted = append(result.Deleted, copyBytes(p.Key))
		}
	}

	// Remaining children in B are added
	for ; j < len(childrenB); j++ {
		pairs, err := d.collectAllPairsFromHash(childrenB[j].Hash)
		if err != nil {
			return err
		}
		for _, p := range pairs {
			result.Added = append(result.Added, copyKVPair(p))
		}
	}

	return nil
}

// collectAllPairs collects all KV pairs from a node recursively
func (d *DiffEngine) collectAllPairs(node types.Node) ([]types.KVPair, error) {
	if node.IsLeaf() {
		leaf := node.(*types.LeafNode)
		return leaf.Pairs, nil
	}

	internal := node.(*types.InternalNode)
	var allPairs []types.KVPair
	for _, child := range internal.Children {
		childNode, err := d.loadNode(child.Hash)
		if err != nil {
			return nil, err
		}
		pairs, err := d.collectAllPairs(childNode)
		if err != nil {
			return nil, err
		}
		allPairs = append(allPairs, pairs...)
	}
	return allPairs, nil
}

// collectAllPairsFromHash loads a node and collects all its pairs
func (d *DiffEngine) collectAllPairsFromHash(hash types.Hash) ([]types.KVPair, error) {
	node, err := d.loadNode(hash)
	if err != nil {
		return nil, err
	}
	return d.collectAllPairs(node)
}

// collectPairsInRange collects pairs from a subtree (used for complex overlap cases)
func (d *DiffEngine) collectPairsInRange(hash types.Hash, lower, upper []byte) ([]types.KVPair, error) {
	// For simplicity, collect all pairs - the caller will filter as needed
	return d.collectAllPairsFromHash(hash)
}

// copyBytes creates a copy of a byte slice
func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// copyKVPair creates a deep copy of a KVPair
func copyKVPair(p types.KVPair) types.KVPair {
	return types.KVPair{
		Key:   copyBytes(p.Key),
		Value: copyBytes(p.Value),
	}
}
