package store

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"microprolly/pkg/cas"
	"microprolly/pkg/chunker"
	"microprolly/pkg/tree"
	"microprolly/pkg/types"
)

var (
	// ErrKeyNotFound is returned when a key does not exist in the store
	ErrKeyNotFound = errors.New("key not found")
	// ErrCommitNotFound is returned when a commit does not exist
	ErrCommitNotFound = errors.New("commit not found")
	// ErrInvalidKey is returned when an empty key is provided
	ErrInvalidKey = errors.New("invalid key: empty keys not allowed")
)

// Store is the main user-facing interface for the versioned key-value store
type Store struct {
	mu sync.RWMutex

	// Storage layer
	cas cas.CAS

	// Tree layer
	builder   *tree.TreeBuilder
	traverser *tree.TreeTraverser
	differ    *tree.DiffEngine

	// Version layer
	commitMgr *CommitManager

	// Working state - in-memory map of current uncommitted changes
	workingState map[string][]byte

	// HEAD commit reference
	head types.Hash

	// Data directory for HEAD file persistence
	dataDir string
}

// NewStore creates a new Store with the given CAS directory
// Requirements: 9.1, 9.2
func NewStore(dataDir string) (*Store, error) {
	// Initialize CAS
	casStore, err := cas.NewFileCAS(dataDir)
	if err != nil {
		return nil, err
	}

	store := NewStoreWithCAS(casStore)
	store.dataDir = dataDir

	// Load HEAD from file if it exists
	if err := store.loadHead(); err != nil {
		// If HEAD file doesn't exist, that's fine - start with ZeroHash
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// If we have a HEAD commit, load its state into working state
	if store.head != ZeroHash {
		if err := store.loadWorkingStateFromHead(); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// NewStoreWithCAS creates a new Store with an existing CAS instance
func NewStoreWithCAS(casStore cas.CAS) *Store {
	chunkr := chunker.DefaultChunker()

	return &Store{
		cas:          casStore,
		builder:      tree.NewTreeBuilder(casStore, chunkr),
		traverser:    tree.NewTreeTraverser(casStore),
		differ:       tree.NewDiffEngine(casStore),
		commitMgr:    NewCommitManager(casStore),
		workingState: make(map[string][]byte),
		head:         ZeroHash,
	}
}

// Put stores a key-value pair in the working state
// Requirements: 1.1
func (s *Store) Put(key, value []byte) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Store in working state (make copies to avoid external mutation)
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	s.workingState[string(keyCopy)] = valueCopy
	return nil
}

// Get retrieves a value from the current working state
// Requirements: 1.2, 1.3
func (s *Store) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrInvalidKey
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.workingState[string(key)]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Return a copy to avoid external mutation
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Delete removes a key from the working state
// Requirements: 1.4, 1.5
func (s *Store) Delete(key []byte) error {
	if len(key) == 0 {
		return ErrInvalidKey
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workingState[string(key)]; !exists {
		return ErrKeyNotFound
	}

	delete(s.workingState, string(key))
	return nil
}

// Head returns the current HEAD commit hash
func (s *Store) Head() types.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.head
}

// workingStateToSortedPairs converts the working state map to sorted KV pairs
func (s *Store) workingStateToSortedPairs() []types.KVPair {
	pairs := make([]types.KVPair, 0, len(s.workingState))
	for k, v := range s.workingState {
		pairs = append(pairs, types.KVPair{
			Key:   []byte(k),
			Value: v,
		})
	}

	// Sort by key for deterministic tree construction
	sort.Slice(pairs, func(i, j int) bool {
		return bytes.Compare(pairs[i].Key, pairs[j].Key) < 0
	})

	return pairs
}

// Close releases resources
func (s *Store) Close() error {
	return s.cas.Close()
}

// headFilePath returns the path to the HEAD file
func (s *Store) headFilePath() string {
	return filepath.Join(s.dataDir, "HEAD")
}

// loadHead loads the HEAD commit hash from the HEAD file
// Requirements: 9.2
func (s *Store) loadHead() error {
	if s.dataDir == "" {
		return nil // No persistence for in-memory stores
	}

	data, err := os.ReadFile(s.headFilePath())
	if err != nil {
		return err
	}

	// HEAD file contains hex-encoded hash
	hashStr := string(bytes.TrimSpace(data))
	if hashStr == "" {
		s.head = ZeroHash
		return nil
	}

	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil {
		return err
	}

	if len(hashBytes) != 32 {
		return errors.New("invalid HEAD file: hash must be 32 bytes")
	}

	copy(s.head[:], hashBytes)
	return nil
}

// saveHead persists the HEAD commit hash to the HEAD file
// Requirements: 9.2, 9.3
func (s *Store) saveHead() error {
	if s.dataDir == "" {
		return nil // No persistence for in-memory stores
	}

	headPath := s.headFilePath()

	// Atomic write: write to temp file, sync, then rename
	tmpFile, err := os.CreateTemp(s.dataDir, ".HEAD-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Write hex-encoded hash
	_, err = tmpFile.WriteString(s.head.String() + "\n")
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	// Sync to ensure data is written to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, headPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// loadWorkingStateFromHead loads the working state from the current HEAD commit
func (s *Store) loadWorkingStateFromHead() error {
	if s.head == ZeroHash {
		return nil
	}

	// Get the commit to find its root hash
	commit, err := s.commitMgr.GetCommit(s.head)
	if err != nil {
		return err
	}

	// Load all KV pairs from the commit's tree
	pairs, err := s.traverser.GetAll(commit.RootHash)
	if err != nil {
		return err
	}

	// Populate working state with commit's data
	s.workingState = make(map[string][]byte)
	for _, pair := range pairs {
		keyCopy := make([]byte, len(pair.Key))
		copy(keyCopy, pair.Key)
		valueCopy := make([]byte, len(pair.Value))
		copy(valueCopy, pair.Value)
		s.workingState[string(keyCopy)] = valueCopy
	}

	return nil
}

// Commit creates a new commit with the current working state
// Requirements: 5.1, 5.2, 9.2
func (s *Store) Commit(message string) (types.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert working state to sorted KV pairs
	pairs := s.workingStateToSortedPairs()

	// Build Prolly Tree from working state
	rootHash, err := s.builder.Build(pairs)
	if err != nil {
		return types.Hash{}, err
	}

	// Create commit with tree root hash
	_, commitHash, err := s.commitMgr.CreateCommit(rootHash, message, s.head)
	if err != nil {
		return types.Hash{}, err
	}

	// Update HEAD reference
	s.head = commitHash

	// Persist HEAD to disk
	if err := s.saveHead(); err != nil {
		return types.Hash{}, err
	}

	return commitHash, nil
}

// GetAt retrieves a value as it existed at a specific commit
// Requirements: 6.1, 6.2, 6.3
func (s *Store) GetAt(key []byte, commitHash types.Hash) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrInvalidKey
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get the commit to find its root hash
	commit, err := s.commitMgr.GetCommit(commitHash)
	if err != nil {
		return nil, ErrCommitNotFound
	}

	// Traverse the tree from that commit's root
	value, err := s.traverser.Get(commit.RootHash, key)
	if err != nil {
		if err == tree.ErrKeyNotFound {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}

	return value, nil
}

// Checkout sets the working state to match a specific commit's data
// Requirements: 6.4, 9.2
func (s *Store) Checkout(commitHash types.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the commit to find its root hash
	commit, err := s.commitMgr.GetCommit(commitHash)
	if err != nil {
		return ErrCommitNotFound
	}

	// Load all KV pairs from the commit's tree
	pairs, err := s.traverser.GetAll(commit.RootHash)
	if err != nil {
		return err
	}

	// Clear current working state and populate with commit's data
	s.workingState = make(map[string][]byte)
	for _, pair := range pairs {
		// Make copies to avoid external mutation
		keyCopy := make([]byte, len(pair.Key))
		copy(keyCopy, pair.Key)
		valueCopy := make([]byte, len(pair.Value))
		copy(valueCopy, pair.Value)
		s.workingState[string(keyCopy)] = valueCopy
	}

	// Update HEAD reference
	s.head = commitHash

	// Persist HEAD to disk
	if err := s.saveHead(); err != nil {
		return err
	}

	return nil
}

// Diff compares two commits and returns the differences
// Requirements: 7.1
func (s *Store) Diff(hashA, hashB types.Hash) (tree.DiffResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get both commits to find their root hashes
	commitA, err := s.commitMgr.GetCommit(hashA)
	if err != nil {
		return tree.DiffResult{}, ErrCommitNotFound
	}

	commitB, err := s.commitMgr.GetCommit(hashB)
	if err != nil {
		return tree.DiffResult{}, ErrCommitNotFound
	}

	// Use the diff engine to compare the trees
	return s.differ.Diff(commitA.RootHash, commitB.RootHash)
}

// Log returns the commit history from the current HEAD
// Requirements: 5.3
func (s *Store) Log() ([]*types.Commit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.head == ZeroHash {
		return []*types.Commit{}, nil
	}

	return s.commitMgr.Log(s.head)
}
