package branch

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"microprolly/pkg/types"
)

var (
	// ErrBranchExists is returned when attempting to create a branch that already exists
	ErrBranchExists = errors.New("branch already exists")
	// ErrBranchNotFound is returned when a branch does not exist
	ErrBranchNotFound = errors.New("branch not found")
	// ErrBranchPathConflict is returned when a branch name conflicts with an existing path
	ErrBranchPathConflict = errors.New("branch name conflicts with existing branch path")
)

// BranchManager handles branch operations
type BranchManager struct {
	refsDir string // Path to refs/heads/ directory
}

// NewBranchManager creates a new BranchManager
func NewBranchManager(dataDir string) (*BranchManager, error) {
	refsDir := filepath.Join(dataDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return nil, err
	}
	return &BranchManager{refsDir: refsDir}, nil
}

// branchFilePath returns the path to a branch reference file
func (bm *BranchManager) branchFilePath(name string) string {
	return filepath.Join(bm.refsDir, name)
}

// CreateBranch creates a new branch pointing to the given commit
// Requirements: 1.1, 1.2, 1.5
func (bm *BranchManager) CreateBranch(name string, commitHash types.Hash) error {
	// Validate branch name
	if err := ValidateBranchName(name); err != nil {
		return err
	}

	// Check if branch already exists
	if bm.BranchExists(name) {
		return ErrBranchExists
	}

	// Check for path conflicts
	if err := bm.checkPathConflict(name); err != nil {
		return err
	}

	// Write branch reference atomically
	return bm.writeBranchRef(name, commitHash)
}

// checkPathConflict checks if creating a branch would conflict with existing branches
// For example, if "foo" exists as a branch (file), we can't create "foo/bar"
// And if "foo/bar" exists, we can't create "foo"
func (bm *BranchManager) checkPathConflict(name string) error {
	branchPath := bm.branchFilePath(name)

	// Check if any parent path component is an existing branch (file)
	// This prevents creating "foo/bar" when "foo" exists as a branch
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		parentName := strings.Join(parts[:i], "/")
		parentPath := bm.branchFilePath(parentName)
		info, err := os.Stat(parentPath)
		if err == nil && !info.IsDir() {
			return ErrBranchPathConflict
		}
	}

	// Check if the target path is a directory (meaning nested branches exist under it)
	// This prevents creating "foo" when "foo/bar" exists
	info, err := os.Stat(branchPath)
	if err == nil && info.IsDir() {
		return ErrBranchPathConflict
	}

	return nil
}

// GetBranch returns the commit hash a branch points to
// Requirements: 2.2
func (bm *BranchManager) GetBranch(name string) (types.Hash, error) {
	branchPath := bm.branchFilePath(name)

	data, err := os.ReadFile(branchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return types.Hash{}, ErrBranchNotFound
		}
		return types.Hash{}, err
	}

	// Parse the hash from the file
	hashStr := strings.TrimSpace(string(data))
	return parseHash(hashStr)
}

// BranchExists checks if a branch exists
func (bm *BranchManager) BranchExists(name string) bool {
	branchPath := bm.branchFilePath(name)
	_, err := os.Stat(branchPath)
	return err == nil
}

// writeBranchRef writes a branch reference file atomically
func (bm *BranchManager) writeBranchRef(name string, commitHash types.Hash) error {
	branchPath := bm.branchFilePath(name)

	// Ensure parent directory exists (for nested branch names like feature/foo)
	dir := filepath.Dir(branchPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Atomic write: write to temp file, sync, then rename
	tmpFile, err := os.CreateTemp(dir, ".branch-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Write hex-encoded hash
	_, err = tmpFile.WriteString(commitHash.String() + "\n")
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
	if err := os.Rename(tmpPath, branchPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// ListBranches returns all branch names
// Requirements: 2.1
func (bm *BranchManager) ListBranches() ([]string, error) {
	var branches []string

	err := filepath.Walk(bm.refsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip temp files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Get relative path from refs/heads/
		relPath, err := filepath.Rel(bm.refsDir, path)
		if err != nil {
			return err
		}

		branches = append(branches, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return branches, nil
}

// DeleteBranch removes a branch reference
// Requirements: 4.1, 4.4
func (bm *BranchManager) DeleteBranch(name string) error {
	// Check if branch exists
	if !bm.BranchExists(name) {
		return ErrBranchNotFound
	}

	branchPath := bm.branchFilePath(name)
	if err := os.Remove(branchPath); err != nil {
		return err
	}

	// Clean up empty parent directories (for nested branch names)
	dir := filepath.Dir(branchPath)
	for dir != bm.refsDir {
		// Try to remove the directory - will fail if not empty
		if err := os.Remove(dir); err != nil {
			break // Directory not empty or other error, stop
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

// UpdateBranch updates a branch to point to a new commit
// Requirements: 5.3
func (bm *BranchManager) UpdateBranch(name string, commitHash types.Hash) error {
	// Check if branch exists
	if !bm.BranchExists(name) {
		return ErrBranchNotFound
	}

	// Write the new reference atomically
	return bm.writeBranchRef(name, commitHash)
}

// parseHash parses a hex-encoded hash string into a types.Hash
func parseHash(hashStr string) (types.Hash, error) {
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil {
		return types.Hash{}, err
	}

	if len(hashBytes) != 32 {
		return types.Hash{}, errors.New("invalid hash: must be 32 bytes")
	}

	var hash types.Hash
	copy(hash[:], hashBytes)
	return hash, nil
}
