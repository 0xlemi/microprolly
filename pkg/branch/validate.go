package branch

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidBranchName is returned when a branch name is invalid
	ErrInvalidBranchName = errors.New("invalid branch name")
	// ErrBranchNameEmpty is returned when a branch name is empty
	ErrBranchNameEmpty = errors.New("branch name cannot be empty")
	// ErrBranchNameReserved is returned when a branch name is reserved
	ErrBranchNameReserved = errors.New("branch name is reserved")
)

// invalidChars contains characters that are not allowed in branch names
var invalidChars = []rune{' ', '~', '^', ':', '?', '*', '[', '\\'}

// ValidateBranchName validates a branch name according to the rules:
// - Must be non-empty
// - Cannot contain spaces, ~, ^, :, ?, *, [, \
// - Cannot start with - or .
// - Cannot end with .lock
// - Cannot contain .. or //
// - Cannot be the reserved name HEAD
func ValidateBranchName(name string) error {
	// Check for empty name
	if name == "" {
		return ErrBranchNameEmpty
	}

	// Check for reserved name
	if name == "HEAD" {
		return ErrBranchNameReserved
	}

	// Check for invalid starting characters
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return ErrInvalidBranchName
	}

	// Check for invalid ending
	if strings.HasSuffix(name, ".lock") {
		return ErrInvalidBranchName
	}

	// Check for invalid sequences
	if strings.Contains(name, "..") || strings.Contains(name, "//") {
		return ErrInvalidBranchName
	}

	// Check for invalid characters
	for _, char := range invalidChars {
		if strings.ContainsRune(name, char) {
			return ErrInvalidBranchName
		}
	}

	return nil
}
