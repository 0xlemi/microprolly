package cas

import (
	"sync"

	"microprolly/pkg/types"
)

// WriteStats tracks statistics about CAS write operations
type WriteStats struct {
	// TotalWrites is the total number of Write calls
	TotalWrites int
	// ActualWrites is the number of writes that actually stored new data (not deduplicated)
	ActualWrites int
	// DeduplicatedWrites is the number of writes that were skipped due to existing data
	DeduplicatedWrites int
	// WrittenHashes contains all hashes that were actually written (new data)
	WrittenHashes []types.Hash
	// AllHashes contains all hashes from Write calls (including deduplicated)
	AllHashes []types.Hash
}

// TrackingCAS wraps a CAS implementation to track write operations
// This is useful for verifying structural sharing efficiency
type TrackingCAS struct {
	inner CAS
	mu    sync.Mutex
	stats WriteStats
}

// NewTrackingCAS creates a new TrackingCAS wrapping the given CAS
func NewTrackingCAS(inner CAS) *TrackingCAS {
	return &TrackingCAS{
		inner: inner,
		stats: WriteStats{
			WrittenHashes: make([]types.Hash, 0),
			AllHashes:     make([]types.Hash, 0),
		},
	}
}

// Write stores data and returns its SHA-256 hash, tracking the operation
func (t *TrackingCAS) Write(data []byte) (types.Hash, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if data already exists before writing
	existedBefore := t.inner.Exists(hashData(data))

	// Perform the actual write
	hash, err := t.inner.Write(data)
	if err != nil {
		return types.Hash{}, err
	}

	// Track statistics
	t.stats.TotalWrites++
	t.stats.AllHashes = append(t.stats.AllHashes, hash)

	if existedBefore {
		t.stats.DeduplicatedWrites++
	} else {
		t.stats.ActualWrites++
		t.stats.WrittenHashes = append(t.stats.WrittenHashes, hash)
	}

	return hash, nil
}

// Read retrieves data by its hash
func (t *TrackingCAS) Read(hash types.Hash) ([]byte, error) {
	return t.inner.Read(hash)
}

// Exists checks if a hash exists in storage
func (t *TrackingCAS) Exists(hash types.Hash) bool {
	return t.inner.Exists(hash)
}

// Close releases resources
func (t *TrackingCAS) Close() error {
	return t.inner.Close()
}

// Stats returns a copy of the current write statistics
func (t *TrackingCAS) Stats() WriteStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Return a copy to avoid race conditions
	statsCopy := WriteStats{
		TotalWrites:        t.stats.TotalWrites,
		ActualWrites:       t.stats.ActualWrites,
		DeduplicatedWrites: t.stats.DeduplicatedWrites,
		WrittenHashes:      make([]types.Hash, len(t.stats.WrittenHashes)),
		AllHashes:          make([]types.Hash, len(t.stats.AllHashes)),
	}
	copy(statsCopy.WrittenHashes, t.stats.WrittenHashes)
	copy(statsCopy.AllHashes, t.stats.AllHashes)

	return statsCopy
}

// ResetStats clears all tracked statistics
func (t *TrackingCAS) ResetStats() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats = WriteStats{
		WrittenHashes: make([]types.Hash, 0),
		AllHashes:     make([]types.Hash, 0),
	}
}

// hashData computes the SHA-256 hash of data without storing it
func hashData(data []byte) types.Hash {
	return types.HashFromBytes(data)
}

// CountUniqueHashes returns the number of unique hashes in the given slice
func CountUniqueHashes(hashes []types.Hash) int {
	seen := make(map[types.Hash]bool)
	for _, h := range hashes {
		seen[h] = true
	}
	return len(seen)
}

// HashIntersection returns hashes that appear in both slices
func HashIntersection(a, b []types.Hash) []types.Hash {
	setA := make(map[types.Hash]bool)
	for _, h := range a {
		setA[h] = true
	}

	result := make([]types.Hash, 0)
	seen := make(map[types.Hash]bool)
	for _, h := range b {
		if setA[h] && !seen[h] {
			result = append(result, h)
			seen[h] = true
		}
	}
	return result
}
