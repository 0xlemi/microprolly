package tree

import (
	"bytes"
	"errors"

	"microprolly/pkg/cas"
	"microprolly/pkg/types"
)

var (
	// ErrKeyNotFound is returned when a key does not exist in the tree
	ErrKeyNotFound = errors.New("key not found")
)

// TreeTraverser provides tree navigation operations
type TreeTraverser struct {
	cas cas.CAS
}

// NewTreeTraverser creates a new TreeTraverser with the given CAS
func NewTreeTraverser(cas cas.CAS) *TreeTraverser {
	return &TreeTraverser{cas: cas}
}

// Get retrieves a value by key from a tree rooted at the given hash.
// It traverses from root to leaf using binary search within nodes.
// Nodes are loaded from CAS on demand.
// Requirement 3.5: O(log n) time complexity
func (t *TreeTraverser) Get(rootHash types.Hash, key []byte) ([]byte, error) {
	// Load the root node
	node, err := t.loadNode(rootHash)
	if err != nil {
		return nil, err
	}

	// Traverse down the tree until we reach a leaf
	for !node.IsLeaf() {
		internal := node.(*types.InternalNode)

		// Find the appropriate child using binary search
		childHash := t.findChild(internal, key)

		// Load the child node
		node, err = t.loadNode(childHash)
		if err != nil {
			return nil, err
		}
	}

	// We're at a leaf node - search for the key
	leaf := node.(*types.LeafNode)
	return t.searchLeaf(leaf, key)
}

// loadNode loads a node from CAS by its hash
func (t *TreeTraverser) loadNode(hash types.Hash) (types.Node, error) {
	data, err := t.cas.Read(hash)
	if err != nil {
		return nil, err
	}
	return DeserializeNode(data)
}

// findChild finds the appropriate child hash for a given key in an internal node.
// Uses binary search to find the last child whose key is <= the search key.
// Children are sorted by key, and each child's key represents the minimum key in that subtree.
func (t *TreeTraverser) findChild(node *types.InternalNode, key []byte) types.Hash {
	children := node.Children

	// Binary search to find the rightmost child whose key is <= search key
	// We want the largest index i where children[i].Key <= key
	lo, hi := 0, len(children)-1
	result := 0 // Default to first child

	for lo <= hi {
		mid := (lo + hi) / 2
		cmp := bytes.Compare(children[mid].Key, key)

		if cmp <= 0 {
			// children[mid].Key <= key, this could be our answer
			result = mid
			lo = mid + 1
		} else {
			// children[mid].Key > key, search left
			hi = mid - 1
		}
	}

	return children[result].Hash
}

// searchLeaf searches for a key in a leaf node using binary search.
// Returns the value if found, or ErrKeyNotFound if not present.
func (t *TreeTraverser) searchLeaf(leaf *types.LeafNode, key []byte) ([]byte, error) {
	pairs := leaf.Pairs

	// Binary search for the key
	lo, hi := 0, len(pairs)-1

	for lo <= hi {
		mid := (lo + hi) / 2
		cmp := bytes.Compare(pairs[mid].Key, key)

		if cmp == 0 {
			// Found the key
			return pairs[mid].Value, nil
		} else if cmp < 0 {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	return nil, ErrKeyNotFound
}

// GetAll returns all KV pairs in the tree in sorted order.
// It performs a full tree traversal, visiting all leaf nodes from left to right.
// Requirement 3.5
func (t *TreeTraverser) GetAll(rootHash types.Hash) ([]types.KVPair, error) {
	// Load the root node
	node, err := t.loadNode(rootHash)
	if err != nil {
		return nil, err
	}

	// Collect all pairs via recursive traversal
	var pairs []types.KVPair
	err = t.collectPairs(node, &pairs)
	if err != nil {
		return nil, err
	}

	return pairs, nil
}

// collectPairs recursively collects all KV pairs from a node and its descendants.
// For leaf nodes, it appends all pairs directly.
// For internal nodes, it recursively visits all children in order.
func (t *TreeTraverser) collectPairs(node types.Node, pairs *[]types.KVPair) error {
	if node.IsLeaf() {
		leaf := node.(*types.LeafNode)
		*pairs = append(*pairs, leaf.Pairs...)
		return nil
	}

	// Internal node - visit all children in order
	internal := node.(*types.InternalNode)
	for _, child := range internal.Children {
		childNode, err := t.loadNode(child.Hash)
		if err != nil {
			return err
		}
		if err := t.collectPairs(childNode, pairs); err != nil {
			return err
		}
	}

	return nil
}
