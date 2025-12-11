package tree

import (
	"microprolly/pkg/cas"
	"microprolly/pkg/chunker"
	"microprolly/pkg/types"
)

// TreeBuilder constructs Prolly Trees from sorted KV pairs
type TreeBuilder struct {
	cas     cas.CAS
	chunker chunker.Chunker
}

// NewTreeBuilder creates a new TreeBuilder with the given CAS and chunker
func NewTreeBuilder(cas cas.CAS, chunker chunker.Chunker) *TreeBuilder {
	return &TreeBuilder{
		cas:     cas,
		chunker: chunker,
	}
}

// Build creates a Prolly Tree from sorted KV pairs and returns the root hash.
// The tree is built bottom-up:
// 1. Chunk the KV pairs using rolling hash boundaries
// 2. Create leaf nodes from each chunk
// 3. Recursively build internal nodes until a single root remains
// All nodes are stored in CAS during construction.
func (b *TreeBuilder) Build(pairs []types.KVPair) (types.Hash, error) {
	// Handle empty input
	if len(pairs) == 0 {
		// Create an empty leaf node as root
		emptyLeaf := &types.LeafNode{Pairs: []types.KVPair{}}
		return b.storeNode(emptyLeaf)
	}

	// Step 1: Chunk the KV pairs using rolling hash boundaries (Requirement 3.1)
	chunks := b.chunker.Chunk(pairs)

	// Step 2: Build leaf nodes from chunks
	leafRefs, err := b.buildLeafNodes(chunks)
	if err != nil {
		return types.Hash{}, err
	}

	// Step 3: Recursively build internal nodes until single root (Requirement 3.3)
	return b.buildInternalLayers(leafRefs)
}

// buildLeafNodes creates leaf nodes from chunks and stores them in CAS
func (b *TreeBuilder) buildLeafNodes(chunks [][]types.KVPair) ([]types.ChildRef, error) {
	refs := make([]types.ChildRef, 0, len(chunks))

	for _, chunk := range chunks {
		// Create leaf node
		leaf := &types.LeafNode{Pairs: chunk}

		// Store in CAS and get hash (Requirement 3.2)
		hash, err := b.storeNode(leaf)
		if err != nil {
			return nil, err
		}

		// Create child reference using first key of the chunk
		ref := types.ChildRef{
			Key:  chunk[0].Key,
			Hash: hash,
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

// buildInternalLayers recursively builds internal node layers until a single root
func (b *TreeBuilder) buildInternalLayers(childRefs []types.ChildRef) (types.Hash, error) {
	// Base case: single child means we have our root
	if len(childRefs) == 1 {
		return childRefs[0].Hash, nil
	}

	// Chunk the child references to create internal nodes
	// We need to convert ChildRefs to a format the chunker can work with
	internalChunks := b.chunkChildRefs(childRefs)

	// Build internal nodes from chunks
	parentRefs := make([]types.ChildRef, 0, len(internalChunks))

	for _, chunk := range internalChunks {
		// Create internal node
		internal := &types.InternalNode{Children: chunk}

		// Store in CAS
		hash, err := b.storeNode(internal)
		if err != nil {
			return types.Hash{}, err
		}

		// Create parent reference using first key of the chunk
		ref := types.ChildRef{
			Key:  chunk[0].Key,
			Hash: hash,
		}
		parentRefs = append(parentRefs, ref)
	}

	// Recurse to build next layer
	return b.buildInternalLayers(parentRefs)
}

// chunkChildRefs chunks child references using the same rolling hash approach
// This ensures consistent tree structure across versions
func (b *TreeBuilder) chunkChildRefs(refs []types.ChildRef) [][]types.ChildRef {
	if len(refs) == 0 {
		return nil
	}

	// Convert ChildRefs to KVPairs for chunking
	// We use the key and hash as key-value for consistent chunking
	pairs := make([]types.KVPair, len(refs))
	for i, ref := range refs {
		pairs[i] = types.KVPair{
			Key:   ref.Key,
			Value: ref.Hash[:],
		}
	}

	// Use the chunker to determine boundaries
	kvChunks := b.chunker.Chunk(pairs)

	// Convert back to ChildRef chunks
	result := make([][]types.ChildRef, len(kvChunks))
	refIdx := 0
	for i, kvChunk := range kvChunks {
		result[i] = refs[refIdx : refIdx+len(kvChunk)]
		refIdx += len(kvChunk)
	}

	return result
}

// storeNode serializes a node and stores it in CAS, returning its hash
func (b *TreeBuilder) storeNode(node types.Node) (types.Hash, error) {
	// Serialize the node
	data, err := node.Serialize()
	if err != nil {
		return types.Hash{}, err
	}

	// Store in CAS - this computes SHA-256 hash (Requirement 3.2)
	return b.cas.Write(data)
}
