package branch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"microprolly/pkg/types"
)

const (
	// headRefPrefix is the prefix for branch references in HEAD file
	headRefPrefix = "ref: refs/heads/"
)

var (
	// ErrDetachedHead is returned when an operation requires an attached HEAD
	ErrDetachedHead = errors.New("HEAD is in detached state")
	// ErrInvalidHeadFormat is returned when the HEAD file has an invalid format
	ErrInvalidHeadFormat = errors.New("invalid HEAD file format")
)

// HeadState represents the current HEAD state
type HeadState struct {
	IsDetached bool       // True if HEAD points directly to a commit
	Branch     string     // Branch name if attached (empty if detached)
	CommitHash types.Hash // Commit hash (always set when valid)
}

// HeadManager handles HEAD state operations
type HeadManager struct {
	headFile      string         // Path to HEAD file
	branchManager *BranchManager // Reference to BranchManager for resolving branches
}

// NewHeadManager creates a new HeadManager
func NewHeadManager(dataDir string, branchManager *BranchManager) *HeadManager {
	return &HeadManager{
		headFile:      filepath.Join(dataDir, "HEAD"),
		branchManager: branchManager,
	}
}

// parseHeadFile parses the content of a HEAD file and returns the HeadState
// HEAD file format:
// - Attached: "ref: refs/heads/{branch_name}"
// - Detached: raw 64-character hex hash
// Requirements: 7.1, 7.2
func parseHeadFile(content string, branchManager *BranchManager) (*HeadState, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrInvalidHeadFormat
	}

	// Check if HEAD points to a branch (attached)
	if strings.HasPrefix(content, headRefPrefix) {
		branchName := strings.TrimPrefix(content, headRefPrefix)
		if branchName == "" {
			return nil, ErrInvalidHeadFormat
		}

		// Resolve the branch to get the commit hash
		commitHash, err := branchManager.GetBranch(branchName)
		if err != nil {
			// Branch might not exist yet (e.g., fresh repo with no commits)
			// Return state with zero hash
			if err == ErrBranchNotFound {
				return &HeadState{
					IsDetached: false,
					Branch:     branchName,
					CommitHash: types.Hash{},
				}, nil
			}
			return nil, err
		}

		return &HeadState{
			IsDetached: false,
			Branch:     branchName,
			CommitHash: commitHash,
		}, nil
	}

	// Otherwise, HEAD should be a raw commit hash (detached)
	// Validate it's a valid hex hash (64 characters for SHA-256)
	if len(content) != 64 {
		return nil, ErrInvalidHeadFormat
	}

	commitHash, err := parseHash(content)
	if err != nil {
		return nil, ErrInvalidHeadFormat
	}

	return &HeadState{
		IsDetached: true,
		Branch:     "",
		CommitHash: commitHash,
	}, nil
}

// formatHeadAttached formats HEAD content for attached state
// Requirements: 7.1
func formatHeadAttached(branchName string) string {
	return headRefPrefix + branchName + "\n"
}

// formatHeadDetached formats HEAD content for detached state
// Requirements: 7.2
func formatHeadDetached(commitHash types.Hash) string {
	return commitHash.String() + "\n"
}

// GetHead returns the current HEAD state
// Requirements: 2.4, 7.1, 7.2
func (hm *HeadManager) GetHead() (*HeadState, error) {
	content, err := os.ReadFile(hm.headFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No HEAD file - return default state pointing to main branch
			return &HeadState{
				IsDetached: false,
				Branch:     "main",
				CommitHash: types.Hash{},
			}, nil
		}
		return nil, err
	}

	return parseHeadFile(string(content), hm.branchManager)
}

// GetHeadCommit returns the commit hash that HEAD points to
// If HEAD is attached to a branch, it resolves the branch to get the commit
// Requirements: 7.1, 7.2
func (hm *HeadManager) GetHeadCommit() (types.Hash, error) {
	state, err := hm.GetHead()
	if err != nil {
		return types.Hash{}, err
	}
	return state.CommitHash, nil
}

// SetHeadToBranch sets HEAD to point to a branch (attached state)
// Requirements: 7.1, 7.4
func (hm *HeadManager) SetHeadToBranch(branchName string) error {
	// Validate branch name
	if err := ValidateBranchName(branchName); err != nil {
		return err
	}

	// Verify branch exists
	if !hm.branchManager.BranchExists(branchName) {
		return ErrBranchNotFound
	}

	// Write HEAD file atomically
	content := formatHeadAttached(branchName)
	return hm.writeHeadFile(content)
}

// SetHeadToCommit sets HEAD to point directly to a commit (detached state)
// Requirements: 7.2, 7.3, 7.4
func (hm *HeadManager) SetHeadToCommit(commitHash types.Hash) error {
	content := formatHeadDetached(commitHash)
	return hm.writeHeadFile(content)
}

// writeHeadFile writes the HEAD file atomically
// Requirements: 7.4
func (hm *HeadManager) writeHeadFile(content string) error {
	dir := filepath.Dir(hm.headFile)

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Atomic write: write to temp file, sync, then rename
	tmpFile, err := os.CreateTemp(dir, ".HEAD-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	_, err = tmpFile.WriteString(content)
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
	if err := os.Rename(tmpPath, hm.headFile); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// InitializeHead initializes HEAD to point to the default branch if it doesn't exist
// This is called during store initialization
func (hm *HeadManager) InitializeHead(defaultBranch string) error {
	// Check if HEAD file already exists
	if _, err := os.Stat(hm.headFile); err == nil {
		return nil // HEAD already exists
	}

	// Create HEAD pointing to default branch
	content := formatHeadAttached(defaultBranch)
	return hm.writeHeadFile(content)
}
