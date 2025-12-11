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

// Ensure memoryCAS implements cas.CAS interface
var _ cas.CAS = (*memoryCAS)(nil)

// TestGet_BasicFunctionality tests basic Get operations
func TestGet_BasicFunctionality(t *testing.T) {
	// Create temp directory for CAS
	tmpDir, err := os.MkdirTemp("", "traverser-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create CAS and builder
	storage, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}
	defer storage.Close()

	c := chunker.NewBuzhashChunker(64, 16, 256)
	builder := NewTreeBuilder(storage, c)
	traverser := NewTreeTraverser(storage)

	// Create test data
	pairs := []types.KVPair{
		{Key: []byte("apple"), Value: []byte("red")},
		{Key: []byte("banana"), Value: []byte("yellow")},
		{Key: []byte("cherry"), Value: []byte("red")},
	}

	// Build tree
	rootHash, err := builder.Build(pairs)
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// Test Get for existing keys
	for _, pair := range pairs {
		value, err := traverser.Get(rootHash, pair.Key)
		if err != nil {
			t.Errorf("Get(%s) failed: %v", pair.Key, err)
			continue
		}
		if !bytes.Equal(value, pair.Value) {
			t.Errorf("Get(%s) = %s, want %s", pair.Key, value, pair.Value)
		}
	}

	// Test Get for non-existent key
	_, err = traverser.Get(rootHash, []byte("nonexistent"))
	if err != ErrKeyNotFound {
		t.Errorf("Get(nonexistent) error = %v, want ErrKeyNotFound", err)
	}
}

// TestGetAll_BasicFunctionality tests basic GetAll operations
func TestGetAll_BasicFunctionality(t *testing.T) {
	// Create temp directory for CAS
	tmpDir, err := os.MkdirTemp("", "traverser-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create CAS and builder
	storage, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}
	defer storage.Close()

	c := chunker.NewBuzhashChunker(64, 16, 256)
	builder := NewTreeBuilder(storage, c)
	traverser := NewTreeTraverser(storage)

	// Create test data (already sorted)
	pairs := []types.KVPair{
		{Key: []byte("apple"), Value: []byte("red")},
		{Key: []byte("banana"), Value: []byte("yellow")},
		{Key: []byte("cherry"), Value: []byte("red")},
		{Key: []byte("date"), Value: []byte("brown")},
		{Key: []byte("elderberry"), Value: []byte("purple")},
	}

	// Build tree
	rootHash, err := builder.Build(pairs)
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// Get all pairs
	result, err := traverser.GetAll(rootHash)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	// Verify count
	if len(result) != len(pairs) {
		t.Errorf("GetAll returned %d pairs, want %d", len(result), len(pairs))
	}

	// Verify all pairs match and are in sorted order
	for i, pair := range result {
		if !bytes.Equal(pair.Key, pairs[i].Key) || !bytes.Equal(pair.Value, pairs[i].Value) {
			t.Errorf("GetAll[%d] = {%s, %s}, want {%s, %s}",
				i, pair.Key, pair.Value, pairs[i].Key, pairs[i].Value)
		}
	}
}

// TestGetAll_EmptyTree tests GetAll on an empty tree
func TestGetAll_EmptyTree(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "traverser-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create CAS: %v", err)
	}
	defer storage.Close()

	c := chunker.NewBuzhashChunker(64, 16, 256)
	builder := NewTreeBuilder(storage, c)
	traverser := NewTreeTraverser(storage)

	// Build empty tree
	rootHash, err := builder.Build([]types.KVPair{})
	if err != nil {
		t.Fatalf("Failed to build empty tree: %v", err)
	}

	// Get all pairs
	result, err := traverser.GetAll(rootHash)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("GetAll on empty tree returned %d pairs, want 0", len(result))
	}
}

// memoryCAS is a simple in-memory CAS for fast property testing
type memoryCAS struct {
	data map[types.Hash][]byte
}

func newMemoryCAS() *memoryCAS {
	return &memoryCAS{data: make(map[types.Hash][]byte)}
}

func (m *memoryCAS) Write(data []byte) (types.Hash, error) {
	hash := types.HashFromBytes(data)
	if _, exists := m.data[hash]; !exists {
		m.data[hash] = append([]byte(nil), data...)
	}
	return hash, nil
}

func (m *memoryCAS) Read(hash types.Hash) ([]byte, error) {
	if data, ok := m.data[hash]; ok {
		return data, nil
	}
	return nil, cas.ErrHashNotFound
}

func (m *memoryCAS) Exists(hash types.Hash) bool {
	_, ok := m.data[hash]
	return ok
}

func (m *memoryCAS) Close() error { return nil }

// TestProperty_GetAllReturnsAllPairs verifies that GetAll returns all pairs that were used to build the tree
func TestProperty_GetAllReturnsAllPairs(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		storage := newMemoryCAS()
		c := chunker.NewBuzhashChunker(64, 16, 256)
		builder := NewTreeBuilder(storage, c)
		traverser := NewTreeTraverser(storage)

		// Generate random KV pairs with unique keys
		numPairs := rapid.IntRange(0, 50).Draw(rt, "numPairs")
		pairMap := make(map[string][]byte)
		for i := 0; i < numPairs; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 1, 32).Draw(rt, "key")
			value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")
			pairMap[string(key)] = value
		}

		// Convert to sorted slice
		pairs := make([]types.KVPair, 0, len(pairMap))
		for k, v := range pairMap {
			pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
		}
		sort.Slice(pairs, func(i, j int) bool {
			return bytes.Compare(pairs[i].Key, pairs[j].Key) < 0
		})

		// Build tree
		rootHash, err := builder.Build(pairs)
		if err != nil {
			rt.Fatalf("Failed to build tree: %v", err)
		}

		// GetAll should return all pairs
		result, err := traverser.GetAll(rootHash)
		if err != nil {
			rt.Fatalf("GetAll failed: %v", err)
		}

		// Verify count
		if len(result) != len(pairs) {
			rt.Fatalf("GetAll returned %d pairs, want %d", len(result), len(pairs))
		}

		// Verify all pairs match
		for i, pair := range result {
			if !bytes.Equal(pair.Key, pairs[i].Key) || !bytes.Equal(pair.Value, pairs[i].Value) {
				rt.Fatalf("GetAll[%d] mismatch: got {%x, %x}, want {%x, %x}",
					i, pair.Key, pair.Value, pairs[i].Key, pairs[i].Value)
			}
		}
	})
}

// TestProperty_GetFindsAllKeys verifies that Get can find every key that was used to build the tree
func TestProperty_GetFindsAllKeys(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		storage := newMemoryCAS()
		c := chunker.NewBuzhashChunker(64, 16, 256)
		builder := NewTreeBuilder(storage, c)
		traverser := NewTreeTraverser(storage)

		// Generate random KV pairs with unique keys
		numPairs := rapid.IntRange(1, 50).Draw(rt, "numPairs")
		pairMap := make(map[string][]byte)
		for i := 0; i < numPairs; i++ {
			key := rapid.SliceOfN(rapid.Byte(), 1, 32).Draw(rt, "key")
			value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")
			pairMap[string(key)] = value
		}

		// Convert to sorted slice
		pairs := make([]types.KVPair, 0, len(pairMap))
		for k, v := range pairMap {
			pairs = append(pairs, types.KVPair{Key: []byte(k), Value: v})
		}
		sort.Slice(pairs, func(i, j int) bool {
			return bytes.Compare(pairs[i].Key, pairs[j].Key) < 0
		})

		// Build tree
		rootHash, err := builder.Build(pairs)
		if err != nil {
			rt.Fatalf("Failed to build tree: %v", err)
		}

		// Get should find every key
		for _, pair := range pairs {
			value, err := traverser.Get(rootHash, pair.Key)
			if err != nil {
				rt.Fatalf("Get(%x) failed: %v", pair.Key, err)
			}
			if !bytes.Equal(value, pair.Value) {
				rt.Fatalf("Get(%x) = %x, want %x", pair.Key, value, pair.Value)
			}
		}
	})
}
