package cas

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"

	"microprolly/pkg/types"
)

var (
	// ErrHashNotFound is returned when a hash does not exist in storage
	ErrHashNotFound = errors.New("hash not found in storage")
)

// CAS provides content-addressed storage operations
type CAS interface {
	// Write stores data and returns its SHA-256 hash
	Write(data []byte) (types.Hash, error)

	// Read retrieves data by its hash
	Read(hash types.Hash) ([]byte, error)

	// Exists checks if a hash exists in storage
	Exists(hash types.Hash) bool

	// Close releases resources
	Close() error
}

// FileCAS implements CAS using the file system
type FileCAS struct {
	baseDir string
}

// NewFileCAS creates a new file-based CAS at the given directory
func NewFileCAS(baseDir string) (*FileCAS, error) {
	objectsDir := filepath.Join(baseDir, "objects")
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		return nil, err
	}
	return &FileCAS{baseDir: baseDir}, nil
}

// objectPath returns the path for an object with the given hash
// Uses two-level directory structure: objects/ab/cdef... for scalability
func (c *FileCAS) objectPath(hash types.Hash) string {
	hexHash := hash.String()
	// First two characters form the subdirectory
	return filepath.Join(c.baseDir, "objects", hexHash[:2], hexHash[2:])
}

// Write stores data and returns its SHA-256 hash
// If the data already exists (same hash), it skips writing and returns the existing hash
func (c *FileCAS) Write(data []byte) (types.Hash, error) {
	hash := sha256.Sum256(data)

	// Check if already exists (deduplication)
	if c.Exists(hash) {
		return hash, nil
	}

	objPath := c.objectPath(hash)

	// Create subdirectory if needed
	dir := filepath.Dir(objPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return types.Hash{}, err
	}

	// Atomic write: write to temp file, sync, then rename
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return types.Hash{}, err
	}
	tmpPath := tmpFile.Name()

	// Write data to temp file - write is asynchronous
	_, err = tmpFile.Write(data)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return types.Hash{}, err
	}

	// Sync to ensure data is written to disk before rename - this blocks until is written makes it sync
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return types.Hash{}, err
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return types.Hash{}, err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, objPath); err != nil {
		os.Remove(tmpPath)
		return types.Hash{}, err
	}

	return hash, nil
}

// Read retrieves data by its hash
func (c *FileCAS) Read(hash types.Hash) ([]byte, error) {
	objPath := c.objectPath(hash)

	file, err := os.Open(objPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrHashNotFound
		}
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

// Exists checks if a hash exists in storage
func (c *FileCAS) Exists(hash types.Hash) bool {
	objPath := c.objectPath(hash)
	_, err := os.Stat(objPath)
	return err == nil
}

// Close releases resources (no-op for file-based CAS)
func (c *FileCAS) Close() error {
	return nil
}
