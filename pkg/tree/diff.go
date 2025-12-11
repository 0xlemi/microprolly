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
//
// The algorithm handles the complex case where tree structure changes due to
// content-defined chunking. When a single key is added/deleted, chunk boundaries
// may shift, causing children to have different starting keys.
//
// Strategy: We use a merge-like algorithm that tracks which key ranges have been
// processed. When children align (same starting key), we can compare them directly.
// When they don't align, we need to carefully track which ranges overlap.
func (d *DiffEngine) diffInternalNodes(nodeA, nodeB *types.InternalNode, result *DiffResult) error {
	childrenA := nodeA.Children
	childrenB := nodeB.Children

	// Build a list of all unique boundary keys from both trees
	// This helps us understand the key space partitioning
	allKeys := make(map[string]bool)
	for _, c := range childrenA {
		allKeys[string(c.Key)] = true
	}
	for _, c := range childrenB {
		allKeys[string(c.Key)] = true
	}

	// If children align perfectly (same keys), use optimized path
	if len(childrenA) == len(childrenB) {
		aligned := true
		for i := range childrenA {
			if !bytes.Equal(childrenA[i].Key, childrenB[i].Key) {
				aligned = false
				break
			}
		}
		if aligned {
			return d.diffAlignedChildren(childrenA, childrenB, result)
		}
	}

	// Children don't align - fall back to collecting all pairs and comparing
	// This is correct but less efficient for the misaligned case
	pairsA, err := d.collectAllPairsFromChildren(childrenA)
	if err != nil {
		return err
	}
	pairsB, err := d.collectAllPairsFromChildren(childrenB)
	if err != nil {
		return err
	}
	d.diffPairLists(pairsA, pairsB, result)
	return nil
}

// diffAlignedChildren handles the case where children have the same starting keys
func (d *DiffEngine) diffAlignedChildren(childrenA, childrenB []types.ChildRef, result *DiffResult) error {
	for i := range childrenA {
		if childrenA[i].Hash == childrenB[i].Hash {
			// Requirement 7.3: Skip subtrees with matching hashes
			continue
		}

		// Hashes differ - recursively compare (Requirement 7.4)
		childNodeA, err := d.loadNode(childrenA[i].Hash)
		if err != nil {
			return err
		}
		childNodeB, err := d.loadNode(childrenB[i].Hash)
		if err != nil {
			return err
		}
		if err := d.diffNodes(childNodeA, childNodeB, result); err != nil {
			return err
		}
	}
	return nil
}

// collectAllPairsFromChildren collects all KV pairs from a list of children
func (d *DiffEngine) collectAllPairsFromChildren(children []types.ChildRef) ([]types.KVPair, error) {
	var allPairs []types.KVPair
	for _, child := range children {
		pairs, err := d.collectAllPairsFromHash(child.Hash)
		if err != nil {
			return nil, err
		}
		allPairs = append(allPairs, pairs...)
	}
	return allPairs, nil
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
