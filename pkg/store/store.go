package store

import (
	"bytes"
	"errors"
	"sort"
	"sync"

	"microprolly/pkg/branch"
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
	// ErrCannotDeleteCurrentBranch is returned when trying to delete the current branch
	ErrCannotDeleteCurrentBranch = errors.New("cannot delete the current branch")
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

	// Branch layer
	branchMgr *branch.BranchManager
	headMgr   *branch.HeadManager

	// Working state - in-memory map of current uncommitted changes
	workingState map[string][]byte

	// HEAD commit reference (cached from HeadManager)
	head types.Hash

	// Data directory for HEAD file persistence
	dataDir string
}

// NewStore creates a new Store with the given CAS directory
// Requirements: 9.1, 9.2, 6.1, 6.2
func NewStore(dataDir string) (*Store, error) {
	// Initialize CAS
	casStore, err := cas.NewFileCAS(dataDir)
	if err != nil {
		return nil, err
	}

	store := NewStoreWithCAS(casStore)
	store.dataDir = dataDir

	// Initialize BranchManager (creates refs/heads/ directory)
	branchMgr, err := branch.NewBranchManager(dataDir)
	if err != nil {
		return nil, err
	}
	store.branchMgr = branchMgr

	// Initialize HeadManager
	store.headMgr = branch.NewHeadManager(dataDir, branchMgr)

	// Check if this is a fresh store (no branches exist)
	branches, err := branchMgr.ListBranches()
	if err != nil {
		return nil, err
	}

	if len(branches) == 0 {
		// Create default "main" branch pointing to ZeroHash
		// This will be updated when the first commit is made
		if err := branchMgr.CreateBranch("main", ZeroHash); err != nil {
			return nil, err
		}
	}

	// Initialize HEAD to point to main branch if it doesn't exist
	if err := store.headMgr.InitializeHead("main"); err != nil {
		return nil, err
	}

	// Load HEAD state from HeadManager
	headState, err := store.headMgr.GetHead()
	if err != nil {
		return nil, err
	}
	store.head = headState.CommitHash

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
// Requirements: 5.1, 5.2, 5.3, 9.2
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

	// If HeadManager is available, update branch pointer or HEAD
	if s.headMgr != nil {
		headState, err := s.headMgr.GetHead()
		if err != nil {
			return types.Hash{}, err
		}

		if headState.IsDetached {
			// Detached HEAD: only update HEAD to point to new commit
			// Requirements: 5.2
			if err := s.headMgr.SetHeadToCommit(commitHash); err != nil {
				return types.Hash{}, err
			}
		} else {
			// Attached HEAD: update branch to point to new commit
			// Requirements: 5.1, 5.3
			if err := s.branchMgr.UpdateBranch(headState.Branch, commitHash); err != nil {
				return types.Hash{}, err
			}
		}
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
// This puts HEAD in detached state pointing to the commit
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

	// Persist HEAD to disk using HeadManager if available
	if s.headMgr != nil {
		if err := s.headMgr.SetHeadToCommit(commitHash); err != nil {
			return err
		}
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

// CreateBranch creates a new branch at the current HEAD commit
// Requirements: 1.1
func (s *Store) CreateBranch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.branchMgr == nil {
		return errors.New("branch manager not initialized")
	}

	return s.branchMgr.CreateBranch(name, s.head)
}

// CreateBranchAt creates a new branch at a specific commit
// Requirements: 1.2, 1.3
func (s *Store) CreateBranchAt(name string, commitHash types.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.branchMgr == nil {
		return errors.New("branch manager not initialized")
	}

	return s.branchMgr.CreateBranch(name, commitHash)
}

// SwitchBranch switches to a different branch, updating HEAD and working state
// Requirements: 3.1, 3.2, 3.3, 3.4
func (s *Store) SwitchBranch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.branchMgr == nil || s.headMgr == nil {
		return errors.New("branch manager not initialized")
	}

	// Check if branch exists
	if !s.branchMgr.BranchExists(name) {
		return branch.ErrBranchNotFound
	}

	// Get the commit hash the branch points to
	commitHash, err := s.branchMgr.GetBranch(name)
	if err != nil {
		return err
	}

	// Update HEAD to point to the branch (attached state)
	if err := s.headMgr.SetHeadToBranch(name); err != nil {
		return err
	}

	// Update cached head
	s.head = commitHash

	// Load working state from the branch's commit
	if commitHash != ZeroHash {
		// Get the commit to find its root hash
		commit, err := s.commitMgr.GetCommit(commitHash)
		if err != nil {
			return err
		}

		// Load all KV pairs from the commit's tree
		pairs, err := s.traverser.GetAll(commit.RootHash)
		if err != nil {
			return err
		}

		// Clear current working state and populate with commit's data
		s.workingState = make(map[string][]byte)
		for _, pair := range pairs {
			keyCopy := make([]byte, len(pair.Key))
			copy(keyCopy, pair.Key)
			valueCopy := make([]byte, len(pair.Value))
			copy(valueCopy, pair.Value)
			s.workingState[string(keyCopy)] = valueCopy
		}
	} else {
		// Branch points to ZeroHash (no commits yet), clear working state
		s.workingState = make(map[string][]byte)
	}

	return nil
}

// CurrentBranch returns the current branch name and whether HEAD is detached
// Returns (branchName, isDetached, error)
// Requirements: 2.4
func (s *Store) CurrentBranch() (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.headMgr == nil {
		return "", false, errors.New("head manager not initialized")
	}

	headState, err := s.headMgr.GetHead()
	if err != nil {
		return "", false, err
	}

	return headState.Branch, headState.IsDetached, nil
}

// DeleteBranch deletes a branch
// Cannot delete the currently checked-out branch
// Requirements: 4.1, 4.2, 4.3
func (s *Store) DeleteBranch(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.branchMgr == nil || s.headMgr == nil {
		return errors.New("branch manager not initialized")
	}

	// Check if branch exists
	if !s.branchMgr.BranchExists(name) {
		return branch.ErrBranchNotFound
	}

	// Check if this is the current branch
	headState, err := s.headMgr.GetHead()
	if err != nil {
		return err
	}

	if !headState.IsDetached && headState.Branch == name {
		return ErrCannotDeleteCurrentBranch
	}

	return s.branchMgr.DeleteBranch(name)
}

// DetachHead sets HEAD to point directly to a commit (detached state)
// Requirements: 7.3
func (s *Store) DetachHead(commitHash types.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.headMgr == nil {
		return errors.New("head manager not initialized")
	}

	// Verify the commit exists
	_, err := s.commitMgr.GetCommit(commitHash)
	if err != nil {
		return ErrCommitNotFound
	}

	// Set HEAD to detached state
	if err := s.headMgr.SetHeadToCommit(commitHash); err != nil {
		return err
	}

	// Update cached head
	s.head = commitHash

	// Load working state from the commit
	commit, err := s.commitMgr.GetCommit(commitHash)
	if err != nil {
		return err
	}

	pairs, err := s.traverser.GetAll(commit.RootHash)
	if err != nil {
		return err
	}

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

// ListBranches returns all branch names
// Requirements: 2.1
func (s *Store) ListBranches() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.branchMgr == nil {
		return nil, errors.New("branch manager not initialized")
	}

	return s.branchMgr.ListBranches()
}
