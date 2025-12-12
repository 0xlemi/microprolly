package branch

import (
	"microprolly/pkg/types"
	"os"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// createTestHeadManager creates a HeadManager with a temporary directory for testing
func createTestHeadManager(t *testing.T) (*HeadManager, *BranchManager, func()) {
	tmpDir, err := os.MkdirTemp("", "head-test-*")
	if err != nil {
		t.Fatal(err)
	}

	bm, err := NewBranchManager(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	hm := NewHeadManager(tmpDir, bm)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return hm, bm, cleanup
}

// TestProperty_HeadFileFormatCorrectness tests Property 10: HEAD File Format Correctness
// **Feature: branching, Property 10: HEAD File Format Correctness**
// **Validates: Requirements 7.1, 7.2**
//
// For any HEAD state (attached or detached), the HEAD file SHALL contain the correct format
// ("ref: refs/heads/{name}" for attached, raw hash for detached).
func TestProperty_HeadFileFormatCorrectness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh environment for each test
		tmpDir, err := os.MkdirTemp("", "head-format-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		bm, err := NewBranchManager(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create BranchManager: %v", err)
		}

		hm := NewHeadManager(tmpDir, bm)

		// Decide whether to test attached or detached HEAD
		isDetached := rapid.Bool().Draw(rt, "is_detached")

		if isDetached {
			// Test detached HEAD format
			commitHash := genCommitHash().Draw(rt, "commit_hash")

			// Set HEAD to detached state
			err = hm.SetHeadToCommit(commitHash)
			if err != nil {
				rt.Fatalf("SetHeadToCommit failed: %v", err)
			}

			// Read the HEAD file directly
			content, err := os.ReadFile(hm.headFile)
			if err != nil {
				rt.Fatalf("Failed to read HEAD file: %v", err)
			}

			// Verify format: should be raw 64-character hex hash
			contentStr := strings.TrimSpace(string(content))
			if len(contentStr) != 64 {
				rt.Fatalf("Detached HEAD should be 64 hex chars, got %d chars: %q", len(contentStr), contentStr)
			}

			// Verify it matches the commit hash
			if contentStr != commitHash.String() {
				rt.Fatalf("HEAD content mismatch: got %q, want %q", contentStr, commitHash.String())
			}

			// Verify GetHead returns correct state
			state, err := hm.GetHead()
			if err != nil {
				rt.Fatalf("GetHead failed: %v", err)
			}
			if !state.IsDetached {
				rt.Fatalf("Expected detached HEAD, got attached")
			}
			if state.CommitHash != commitHash {
				rt.Fatalf("GetHead hash mismatch: got %s, want %s", state.CommitHash.String(), commitHash.String())
			}
		} else {
			// Test attached HEAD format
			branchName := genValidBranchName().Draw(rt, "branch_name")
			commitHash := genCommitHash().Draw(rt, "commit_hash")

			// Create the branch first
			err = bm.CreateBranch(branchName, commitHash)
			if err != nil {
				rt.Fatalf("CreateBranch failed: %v", err)
			}

			// Set HEAD to attached state
			err = hm.SetHeadToBranch(branchName)
			if err != nil {
				rt.Fatalf("SetHeadToBranch failed: %v", err)
			}

			// Read the HEAD file directly
			content, err := os.ReadFile(hm.headFile)
			if err != nil {
				rt.Fatalf("Failed to read HEAD file: %v", err)
			}

			// Verify format: should be "ref: refs/heads/{branch_name}"
			contentStr := strings.TrimSpace(string(content))
			expectedPrefix := "ref: refs/heads/"
			if !strings.HasPrefix(contentStr, expectedPrefix) {
				rt.Fatalf("Attached HEAD should start with %q, got %q", expectedPrefix, contentStr)
			}

			// Verify branch name matches
			extractedBranch := strings.TrimPrefix(contentStr, expectedPrefix)
			if extractedBranch != branchName {
				rt.Fatalf("Branch name mismatch: got %q, want %q", extractedBranch, branchName)
			}

			// Verify GetHead returns correct state
			state, err := hm.GetHead()
			if err != nil {
				rt.Fatalf("GetHead failed: %v", err)
			}
			if state.IsDetached {
				rt.Fatalf("Expected attached HEAD, got detached")
			}
			if state.Branch != branchName {
				rt.Fatalf("GetHead branch mismatch: got %q, want %q", state.Branch, branchName)
			}
			if state.CommitHash != commitHash {
				rt.Fatalf("GetHead hash mismatch: got %s, want %s", state.CommitHash.String(), commitHash.String())
			}
		}
	})
}

// TestProperty_DetachHeadSetsCorrectState tests Property 11: Detach Head Sets Correct State
// **Feature: branching, Property 11: Detach Head Sets Correct State**
// **Validates: Requirements 7.3**
//
// For any valid commit hash, after DetachHead() (SetHeadToCommit), HEAD SHALL be in
// detached state pointing to that commit.
func TestProperty_DetachHeadSetsCorrectState(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh environment for each test
		tmpDir, err := os.MkdirTemp("", "head-detach-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		bm, err := NewBranchManager(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create BranchManager: %v", err)
		}

		hm := NewHeadManager(tmpDir, bm)

		// Generate a random commit hash
		commitHash := genCommitHash().Draw(rt, "commit_hash")

		// Optionally start from an attached state to test transition
		startAttached := rapid.Bool().Draw(rt, "start_attached")
		if startAttached {
			branchName := genValidBranchName().Draw(rt, "branch_name")
			branchHash := genCommitHash().Draw(rt, "branch_hash")

			// Create branch and attach HEAD to it
			err = bm.CreateBranch(branchName, branchHash)
			if err != nil {
				rt.Fatalf("CreateBranch failed: %v", err)
			}
			err = hm.SetHeadToBranch(branchName)
			if err != nil {
				rt.Fatalf("SetHeadToBranch failed: %v", err)
			}

			// Verify we're attached
			state, err := hm.GetHead()
			if err != nil {
				rt.Fatalf("GetHead failed: %v", err)
			}
			if state.IsDetached {
				rt.Fatalf("Expected attached HEAD before detach")
			}
		}

		// Detach HEAD to the commit
		err = hm.SetHeadToCommit(commitHash)
		if err != nil {
			rt.Fatalf("SetHeadToCommit failed: %v", err)
		}

		// Verify HEAD is now detached
		state, err := hm.GetHead()
		if err != nil {
			rt.Fatalf("GetHead failed after detach: %v", err)
		}

		// Property: HEAD must be in detached state
		if !state.IsDetached {
			rt.Fatalf("Expected detached HEAD after SetHeadToCommit, got attached to %q", state.Branch)
		}

		// Property: HEAD must point to the specified commit
		if state.CommitHash != commitHash {
			rt.Fatalf("HEAD commit mismatch: got %s, want %s", state.CommitHash.String(), commitHash.String())
		}

		// Property: Branch should be empty for detached HEAD
		if state.Branch != "" {
			rt.Fatalf("Detached HEAD should have empty branch, got %q", state.Branch)
		}

		// Verify GetHeadCommit also returns the correct hash
		headCommit, err := hm.GetHeadCommit()
		if err != nil {
			rt.Fatalf("GetHeadCommit failed: %v", err)
		}
		if headCommit != commitHash {
			rt.Fatalf("GetHeadCommit mismatch: got %s, want %s", headCommit.String(), commitHash.String())
		}
	})
}

// TestHeadManager_SetHeadToBranchNotFound tests that setting HEAD to non-existent branch fails
func TestHeadManager_SetHeadToBranchNotFound(t *testing.T) {
	hm, _, cleanup := createTestHeadManager(t)
	defer cleanup()

	err := hm.SetHeadToBranch("nonexistent")
	if err != ErrBranchNotFound {
		t.Fatalf("Expected ErrBranchNotFound, got: %v", err)
	}
}

// TestHeadManager_SetHeadToBranchInvalidName tests that setting HEAD to invalid branch name fails
func TestHeadManager_SetHeadToBranchInvalidName(t *testing.T) {
	hm, _, cleanup := createTestHeadManager(t)
	defer cleanup()

	invalidNames := []string{
		"",
		"HEAD",
		"-starts-with-dash",
	}

	for _, name := range invalidNames {
		err := hm.SetHeadToBranch(name)
		if err == nil {
			t.Errorf("SetHeadToBranch(%q) should have failed", name)
		}
	}
}

// TestHeadManager_GetHeadDefault tests that GetHead returns default state when no HEAD file exists
func TestHeadManager_GetHeadDefault(t *testing.T) {
	hm, _, cleanup := createTestHeadManager(t)
	defer cleanup()

	state, err := hm.GetHead()
	if err != nil {
		t.Fatalf("GetHead failed: %v", err)
	}

	// Default should be attached to "main" branch
	if state.IsDetached {
		t.Fatal("Default HEAD should be attached")
	}
	if state.Branch != "main" {
		t.Fatalf("Default branch should be 'main', got %q", state.Branch)
	}
}

// TestHeadManager_InitializeHead tests HEAD initialization
func TestHeadManager_InitializeHead(t *testing.T) {
	hm, bm, cleanup := createTestHeadManager(t)
	defer cleanup()

	// Create a branch first
	err := bm.CreateBranch("develop", types.Hash{1, 2, 3})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Initialize HEAD to develop
	err = hm.InitializeHead("develop")
	if err != nil {
		t.Fatalf("InitializeHead failed: %v", err)
	}

	// Verify HEAD points to develop
	state, err := hm.GetHead()
	if err != nil {
		t.Fatalf("GetHead failed: %v", err)
	}

	if state.IsDetached {
		t.Fatal("HEAD should be attached after initialization")
	}
	if state.Branch != "develop" {
		t.Fatalf("Expected branch 'develop', got %q", state.Branch)
	}
}

// TestParseHeadFile_InvalidFormats tests that invalid HEAD file formats are rejected
func TestParseHeadFile_InvalidFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "head-parse-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	bm, err := NewBranchManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	invalidContents := []string{
		"",                         // Empty
		"ref: ",                    // Missing branch name
		"ref: refs/heads/",         // Empty branch name
		"abc123",                   // Too short for hash
		"ref: invalid/path/branch", // Wrong ref path
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", // Invalid hex
	}

	for _, content := range invalidContents {
		_, err := parseHeadFile(content, bm)
		if err == nil {
			t.Errorf("parseHeadFile(%q) should have failed", content)
		}
	}
}
