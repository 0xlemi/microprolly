package chunker

import (
	"microprolly/pkg/types"
)

// Chunker splits sorted KV pairs into content-defined chunks using rolling hash
type Chunker interface {
	// Chunk takes sorted KV pairs and returns chunk boundaries
	Chunk(pairs []types.KVPair) [][]types.KVPair
}

// BuzhashChunker implements content-defined chunking using Buzhash rolling hash
type BuzhashChunker struct {
	// TargetSize is the average chunk size (boundary when hash % targetSize == 0)
	TargetSize uint32
	// MinSize prevents tiny chunks
	MinSize uint32
	// MaxSize prevents huge chunks
	MaxSize uint32
}

// DefaultChunker returns a chunker with sensible defaults
func DefaultChunker() *BuzhashChunker {
	return &BuzhashChunker{
		TargetSize: 4096,
		MinSize:    512,
		MaxSize:    16384,
	}
}

// NewBuzhashChunker creates a new BuzhashChunker with the given parameters
func NewBuzhashChunker(targetSize, minSize, maxSize uint32) *BuzhashChunker {
	return &BuzhashChunker{
		TargetSize: targetSize,
		MinSize:    minSize,
		MaxSize:    maxSize,
	}
}

// Chunk splits sorted KV pairs into content-defined chunks.
// The chunking is deterministic: the same input always produces the same chunks.
// Chunk boundaries are determined by the rolling hash of serialized KV pairs.
func (c *BuzhashChunker) Chunk(pairs []types.KVPair) [][]types.KVPair {
	if len(pairs) == 0 {
		return nil
	}

	// Create rolling hash with min/max constraints
	hasher := NewBuzhash(c.TargetSize, c.MinSize, c.MaxSize)

	var chunks [][]types.KVPair
	var currentChunk []types.KVPair

	for _, pair := range pairs {
		// Serialize the KV pair for hashing
		serialized := SerializeKVPair(pair)

		// Feed each byte through the rolling hash
		for _, b := range serialized {
			hasher.Roll(b)
		}

		// Add pair to current chunk
		currentChunk = append(currentChunk, pair)

		// Check if we should create a boundary (hasher handles min/max internally)
		if hasher.IsBoundary() {
			// Finalize current chunk
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			hasher.Reset()
		}
	}

	// Don't forget the last chunk if it has any pairs
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}
