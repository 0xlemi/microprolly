package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"microprolly/pkg/cas"
	"microprolly/pkg/types"
)

// ZeroHash represents an empty/null hash (used for initial commit with no parent)
var ZeroHash = types.Hash{}

// commitJSON is the JSON representation of a Commit
// Hash fields are encoded as hex strings for readability
type commitJSON struct {
	RootHash  string `json:"root_hash"`
	Message   string `json:"message"`
	Parent    string `json:"parent"`
	Timestamp int64  `json:"timestamp"`
}

// MarshalCommit serializes a Commit to JSON bytes
func MarshalCommit(c *types.Commit) ([]byte, error) {
	cj := commitJSON{
		RootHash:  hex.EncodeToString(c.RootHash[:]),
		Message:   c.Message,
		Parent:    hex.EncodeToString(c.Parent[:]),
		Timestamp: c.Timestamp,
	}
	return json.Marshal(cj)
}

// UnmarshalCommit deserializes JSON bytes to a Commit
func UnmarshalCommit(data []byte) (*types.Commit, error) {
	var cj commitJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal commit JSON: %w", err)
	}

	rootHashBytes, err := hex.DecodeString(cj.RootHash)
	if err != nil {
		return nil, fmt.Errorf("invalid root_hash hex: %w", err)
	}
	if len(rootHashBytes) != 32 {
		return nil, fmt.Errorf("root_hash must be 32 bytes, got %d", len(rootHashBytes))
	}

	parentBytes, err := hex.DecodeString(cj.Parent)
	if err != nil {
		return nil, fmt.Errorf("invalid parent hex: %w", err)
	}
	if len(parentBytes) != 32 {
		return nil, fmt.Errorf("parent must be 32 bytes, got %d", len(parentBytes))
	}

	var rootHash, parent types.Hash
	copy(rootHash[:], rootHashBytes)
	copy(parent[:], parentBytes)

	return &types.Commit{
		RootHash:  rootHash,
		Message:   cj.Message,
		Parent:    parent,
		Timestamp: cj.Timestamp,
	}, nil
}

// CommitManager handles commit operations
type CommitManager struct {
	cas cas.CAS
}

// NewCommitManager creates a new CommitManager with the given CAS
func NewCommitManager(c cas.CAS) *CommitManager {
	return &CommitManager{cas: c}
}

// CreateCommit creates a new commit with the given root hash, message, and parent
// Returns the commit object and its hash
func (cm *CommitManager) CreateCommit(rootHash types.Hash, message string, parent types.Hash) (*types.Commit, types.Hash, error) {
	commit := &types.Commit{
		RootHash:  rootHash,
		Message:   message,
		Parent:    parent,
		Timestamp: time.Now().Unix(),
	}

	// Serialize commit to JSON
	data, err := MarshalCommit(commit)
	if err != nil {
		return nil, types.Hash{}, fmt.Errorf("failed to marshal commit: %w", err)
	}

	// Store in CAS
	hash, err := cm.cas.Write(data)
	if err != nil {
		return nil, types.Hash{}, fmt.Errorf("failed to write commit to CAS: %w", err)
	}

	return commit, hash, nil
}

// GetCommit retrieves a commit by its hash
func (cm *CommitManager) GetCommit(hash types.Hash) (*types.Commit, error) {
	data, err := cm.cas.Read(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to read commit from CAS: %w", err)
	}

	commit, err := UnmarshalCommit(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal commit: %w", err)
	}

	return commit, nil
}

// Log returns the chain of commits from the given commit hash to the initial commit
// Commits are returned in reverse chronological order (newest first)
func (cm *CommitManager) Log(hash types.Hash) ([]*types.Commit, error) {
	var commits []*types.Commit

	currentHash := hash
	for currentHash != ZeroHash {
		commit, err := cm.GetCommit(currentHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get commit %s: %w", currentHash.String(), err)
		}

		commits = append(commits, commit)
		currentHash = commit.Parent
	}

	return commits, nil
}
