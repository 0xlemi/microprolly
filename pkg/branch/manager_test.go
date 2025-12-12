package branch

import (
	"os"
	"testing"

	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// createTestBranchManager creates a BranchManager with a temporary directory for testing
func createTestBranchManager(t *testing.T) (*BranchManager, func()) {
	tmpDir, err := os.MkdirTemp("", "branch-test-*")
	if err != nil {
		t.Fatal(err)
	}

	bm, err := NewBranchManager(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return bm, cleanup
}

// genValidBranchName generates valid branch names
func genValidBranchName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		// Generate a base name with valid characters
		base := rapid.StringMatching(`[a-z][a-z0-9_-]{0,19}`).Draw(t, "base")
		if base == "" {
			base = "branch"
		}
		// Optionally add a path component
		if rapid.Bool().Draw(t, "has_path") {
			suffix := rapid.StringMatching(`[a-z][a-z0-9_-]{0,9}`).Draw(t, "suffix")
			if suffix != "" {
				base = base + "/" + suffix
			}
		}
		return base
	})
}

// genCommitHash generates a random commit hash
func genCommitHash() *rapid.Generator[types.Hash] {
	return rapid.Custom(func(t *rapid.T) types.Hash {
		var hash types.Hash
		for i := 0; i < 32; i++ {
			hash[i] = rapid.Byte().Draw(t, "hash_byte")
		}
		return hash
	})
}

// TestProperty_BranchCreationRoundTrip tests Property 1: Branch Creation Round-Trip
// **Feature: branching, Property 1: Branch Creation Round-Trip**
// **Validates: Requirements 1.1, 1.2, 2.2**
//
// For any valid branch name and commit hash, creating a branch and then getting it
// SHALL return the same commit hash.
func TestProperty_BranchCreationRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh BranchManager for each test
		tmpDir, err := os.MkdirTemp("", "branch-roundtrip-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		bm, err := NewBranchManager(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create BranchManager: %v", err)
		}

		branchName := genValidBranchName().Draw(rt, "branch_name")
		commitHash := genCommitHash().Draw(rt, "commit_hash")

		// Create the branch
		err = bm.CreateBranch(branchName, commitHash)
		if err != nil {
			rt.Fatalf("CreateBranch(%q, %s) failed: %v", branchName, commitHash.String(), err)
		}

		// Get the branch back
		retrievedHash, err := bm.GetBranch(branchName)
		if err != nil {
			rt.Fatalf("GetBranch(%q) failed: %v", branchName, err)
		}

		// Verify round-trip: retrieved hash should equal original hash
		if retrievedHash != commitHash {
			rt.Fatalf("Round-trip failed: got %s, want %s", retrievedHash.String(), commitHash.String())
		}
	})
}

// TestBranchManager_CreateBranchDuplicate tests that creating a duplicate branch fails
func TestBranchManager_CreateBranchDuplicate(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	hash := types.Hash{}
	err := bm.CreateBranch("main", hash)
	if err != nil {
		t.Fatalf("First CreateBranch failed: %v", err)
	}

	err = bm.CreateBranch("main", hash)
	if err != ErrBranchExists {
		t.Fatalf("Expected ErrBranchExists, got: %v", err)
	}
}

// TestBranchManager_GetBranchNotFound tests that getting a non-existent branch fails
func TestBranchManager_GetBranchNotFound(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	_, err := bm.GetBranch("nonexistent")
	if err != ErrBranchNotFound {
		t.Fatalf("Expected ErrBranchNotFound, got: %v", err)
	}
}

// TestBranchManager_BranchExists tests the BranchExists method
func TestBranchManager_BranchExists(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	// Branch should not exist initially
	if bm.BranchExists("main") {
		t.Fatal("Branch should not exist initially")
	}

	// Create the branch
	err := bm.CreateBranch("main", types.Hash{})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Branch should exist now
	if !bm.BranchExists("main") {
		t.Fatal("Branch should exist after creation")
	}
}

// TestBranchManager_CreateBranchInvalidName tests that invalid branch names are rejected
func TestBranchManager_CreateBranchInvalidName(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	invalidNames := []string{
		"",
		"HEAD",
		"-starts-with-dash",
		".starts-with-dot",
		"ends-with.lock",
		"has space",
	}

	for _, name := range invalidNames {
		err := bm.CreateBranch(name, types.Hash{})
		if err == nil {
			t.Errorf("CreateBranch(%q) should have failed", name)
		}
	}
}

// TestProperty_BranchListingCompleteness tests Property 2: Branch Listing Completeness
// **Feature: branching, Property 2: Branch Listing Completeness**
// **Validates: Requirements 2.1**
//
// For any set of created branches, ListBranches() SHALL return exactly those branch names
// (no more, no less).
func TestProperty_BranchListingCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh BranchManager for each test
		tmpDir, err := os.MkdirTemp("", "branch-listing-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		bm, err := NewBranchManager(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create BranchManager: %v", err)
		}

		// Generate a set of unique branch names
		numBranches := rapid.IntRange(0, 10).Draw(rt, "num_branches")
		createdBranches := make(map[string]bool)

		for i := 0; i < numBranches; i++ {
			branchName := genValidBranchName().Draw(rt, "branch_name")

			// Skip if we already created this branch
			if createdBranches[branchName] {
				continue
			}

			commitHash := genCommitHash().Draw(rt, "commit_hash")
			err := bm.CreateBranch(branchName, commitHash)
			if err != nil {
				// Path conflicts are expected when mixing flat and nested branch names
				// (e.g., "b" and "b/foo" can't coexist)
				if err == ErrBranchPathConflict || err == ErrBranchExists {
					continue
				}
				rt.Fatalf("CreateBranch(%q) failed: %v", branchName, err)
			}
			createdBranches[branchName] = true
		}

		// List all branches
		listedBranches, err := bm.ListBranches()
		if err != nil {
			rt.Fatalf("ListBranches() failed: %v", err)
		}

		// Convert to map for easy comparison
		listedMap := make(map[string]bool)
		for _, name := range listedBranches {
			listedMap[name] = true
		}

		// Verify completeness: all created branches should be listed
		for name := range createdBranches {
			if !listedMap[name] {
				rt.Fatalf("Branch %q was created but not listed", name)
			}
		}

		// Verify no extras: all listed branches should have been created
		for name := range listedMap {
			if !createdBranches[name] {
				rt.Fatalf("Branch %q was listed but not created", name)
			}
		}

		// Verify count matches
		if len(listedBranches) != len(createdBranches) {
			rt.Fatalf("Branch count mismatch: listed %d, created %d", len(listedBranches), len(createdBranches))
		}
	})
}

// TestProperty_BranchDeletionRemovesBranch tests Property 4: Branch Deletion Removes Branch
// **Feature: branching, Property 4: Branch Deletion Removes Branch**
// **Validates: Requirements 4.1, 4.4**
//
// For any existing branch that is not the current branch, after DeleteBranch(),
// the branch SHALL no longer appear in ListBranches() and GetBranch() SHALL return an error.
func TestProperty_BranchDeletionRemovesBranch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh BranchManager for each test
		tmpDir, err := os.MkdirTemp("", "branch-deletion-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		bm, err := NewBranchManager(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create BranchManager: %v", err)
		}

		// Create a branch
		branchName := genValidBranchName().Draw(rt, "branch_name")
		commitHash := genCommitHash().Draw(rt, "commit_hash")

		err = bm.CreateBranch(branchName, commitHash)
		if err != nil {
			rt.Fatalf("CreateBranch(%q) failed: %v", branchName, err)
		}

		// Verify branch exists
		if !bm.BranchExists(branchName) {
			rt.Fatalf("Branch %q should exist after creation", branchName)
		}

		// Delete the branch
		err = bm.DeleteBranch(branchName)
		if err != nil {
			rt.Fatalf("DeleteBranch(%q) failed: %v", branchName, err)
		}

		// Verify branch no longer exists
		if bm.BranchExists(branchName) {
			rt.Fatalf("Branch %q should not exist after deletion", branchName)
		}

		// Verify GetBranch returns error
		_, err = bm.GetBranch(branchName)
		if err != ErrBranchNotFound {
			rt.Fatalf("GetBranch(%q) should return ErrBranchNotFound after deletion, got: %v", branchName, err)
		}

		// Verify branch not in list
		branches, err := bm.ListBranches()
		if err != nil {
			rt.Fatalf("ListBranches() failed: %v", err)
		}
		for _, name := range branches {
			if name == branchName {
				rt.Fatalf("Branch %q should not appear in ListBranches() after deletion", branchName)
			}
		}
	})
}

// TestBranchManager_DeleteBranchNotFound tests that deleting a non-existent branch fails
func TestBranchManager_DeleteBranchNotFound(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	err := bm.DeleteBranch("nonexistent")
	if err != ErrBranchNotFound {
		t.Fatalf("Expected ErrBranchNotFound, got: %v", err)
	}
}

// TestBranchManager_UpdateBranch tests the UpdateBranch method
func TestBranchManager_UpdateBranch(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	// Create a branch
	initialHash := types.Hash{1, 2, 3}
	err := bm.CreateBranch("main", initialHash)
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Update the branch
	newHash := types.Hash{4, 5, 6}
	err = bm.UpdateBranch("main", newHash)
	if err != nil {
		t.Fatalf("UpdateBranch failed: %v", err)
	}

	// Verify the branch points to the new hash
	retrievedHash, err := bm.GetBranch("main")
	if err != nil {
		t.Fatalf("GetBranch failed: %v", err)
	}

	if retrievedHash != newHash {
		t.Fatalf("Expected hash %s, got %s", newHash.String(), retrievedHash.String())
	}
}

// TestBranchManager_UpdateBranchNotFound tests that updating a non-existent branch fails
func TestBranchManager_UpdateBranchNotFound(t *testing.T) {
	bm, cleanup := createTestBranchManager(t)
	defer cleanup()

	err := bm.UpdateBranch("nonexistent", types.Hash{})
	if err != ErrBranchNotFound {
		t.Fatalf("Expected ErrBranchNotFound, got: %v", err)
	}
}
