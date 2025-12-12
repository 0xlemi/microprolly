package branch

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// genInvalidBranchName generates branch names that are guaranteed to be invalid
func genInvalidBranchName() *rapid.Generator[string] {
	return rapid.OneOf(
		// Empty string
		rapid.Just(""),
		// Reserved name HEAD
		rapid.Just("HEAD"),
		// Starts with -
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			return "-" + s
		}),
		// Starts with .
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			return "." + s
		}),
		// Ends with .lock
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "branch"
			}
			return s + ".lock"
		}),
		// Contains ..
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + ".." + s
		}),
		// Contains //
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "//" + s
		}),
		// Contains space
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + " " + s
		}),
		// Contains ~
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "~" + s
		}),
		// Contains ^
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "^" + s
		}),
		// Contains :
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + ":" + s
		}),
		// Contains ?
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "?" + s
		}),
		// Contains *
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "*" + s
		}),
		// Contains [
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "[" + s
		}),
		// Contains \
		rapid.Map(rapid.StringMatching(`[a-z0-9]+`), func(s string) string {
			if s == "" {
				s = "a"
			}
			return s + "\\" + s
		}),
	)
}

// TestProperty_InvalidBranchNameRejection tests Property 3: Invalid Branch Name Rejection
// **Feature: branching, Property 3: Invalid Branch Name Rejection**
// **Validates: Requirements 1.4**
//
// For any invalid branch name (empty, contains invalid characters, etc.),
// ValidateBranchName() SHALL return an error.
func TestProperty_InvalidBranchNameRejection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		invalidName := genInvalidBranchName().Draw(rt, "invalid_branch_name")

		err := ValidateBranchName(invalidName)
		if err == nil {
			rt.Fatalf("ValidateBranchName(%q) should return an error for invalid name, but got nil", invalidName)
		}
	})
}

// TestValidateBranchName_ValidNames tests that valid branch names are accepted
func TestValidateBranchName_ValidNames(t *testing.T) {
	validNames := []string{
		"main",
		"feature/add-login",
		"bugfix-123",
		"release_v1.0",
		"my-branch",
		"a",
		"feature/nested/path",
	}

	for _, name := range validNames {
		err := ValidateBranchName(name)
		if err != nil {
			t.Errorf("ValidateBranchName(%q) returned error %v, expected nil", name, err)
		}
	}
}

// TestValidateBranchName_InvalidNames tests specific invalid branch names
func TestValidateBranchName_InvalidNames(t *testing.T) {
	testCases := []struct {
		name        string
		expectedErr error
	}{
		{"", ErrBranchNameEmpty},
		{"HEAD", ErrBranchNameReserved},
		{"-starts-with-dash", ErrInvalidBranchName},
		{".starts-with-dot", ErrInvalidBranchName},
		{"ends-with.lock", ErrInvalidBranchName},
		{"has..double-dots", ErrInvalidBranchName},
		{"has//double-slash", ErrInvalidBranchName},
		{"has space", ErrInvalidBranchName},
		{"has~tilde", ErrInvalidBranchName},
		{"has^caret", ErrInvalidBranchName},
		{"has:colon", ErrInvalidBranchName},
		{"has?question", ErrInvalidBranchName},
		{"has*asterisk", ErrInvalidBranchName},
		{"has[bracket", ErrInvalidBranchName},
		{"has\\backslash", ErrInvalidBranchName},
	}

	for _, tc := range testCases {
		err := ValidateBranchName(tc.name)
		if err == nil {
			t.Errorf("ValidateBranchName(%q) returned nil, expected error", tc.name)
		}
	}
}

// isValidBranchName is a helper to check if a name would be valid
func isValidBranchName(name string) bool {
	if name == "" || name == "HEAD" {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".lock") {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, "//") {
		return false
	}
	for _, char := range []rune{' ', '~', '^', ':', '?', '*', '[', '\\'} {
		if strings.ContainsRune(name, char) {
			return false
		}
	}
	return true
}
