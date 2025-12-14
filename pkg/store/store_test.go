package store

import (
	"os"
	"testing"

	"microprolly/pkg/cas"

	"pgregory.net/rapid"
)

// createTestStore creates a Store with a temporary directory for testing
func createTestStore(t *testing.T) (*Store, func()) {
	tmpDir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}

	casStore, err := cas.NewFileCAS(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	store := NewStoreWithCAS(casStore)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// genNonEmptyBytes generates a non-empty byte slice for keys
func genNonEmptyBytes(t *rapid.T, name string) []byte {
	// Generate at least 1 byte, up to 100 bytes
	length := rapid.IntRange(1, 100).Draw(t, name+"_len")
	return rapid.SliceOfN(rapid.Byte(), length, length).Draw(t, name)
}

// TestProperty_KVPutGetRoundTrip tests Property 1: KV Put-Get Round-Trip
// **Feature: versioned-kv-store, Property 1: KV Put-Get Round-Trip**
// **Validates: Requirements 1.1, 1.2**
//
// For any valid key and value, if Put(key, value) succeeds, then Get(key) SHALL return the same value.
func TestProperty_KVPutGetRoundTrip(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a non-empty key and any value
		key := genNonEmptyBytes(rt, "key")
		value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")

		// Put the key-value pair
		err := store.Put(key, value)
		if err != nil {
			rt.Fatalf("Put failed: %v", err)
		}

		// Get the value back
		retrieved, err := store.Get(key)
		if err != nil {
			rt.Fatalf("Get failed: %v", err)
		}

		// Verify round-trip: retrieved value should equal original value
		if len(retrieved) != len(value) {
			rt.Fatalf("Value length mismatch: got %d, want %d", len(retrieved), len(value))
		}
		for i := range value {
			if retrieved[i] != value[i] {
				rt.Fatalf("Value mismatch at byte %d: got %d, want %d", i, retrieved[i], value[i])
			}
		}
	})
}

// TestProperty_KVDeleteRemovesKey tests Property 2: KV Delete Removes Key
// **Feature: versioned-kv-store, Property 2: KV Delete Removes Key**
// **Validates: Requirements 1.4**
//
// For any key that exists in the store, after Delete(key) succeeds, Get(key) SHALL return a "key not found" error.
func TestProperty_KVDeleteRemovesKey(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a non-empty key and any value
		key := genNonEmptyBytes(rt, "key")
		value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")

		// First, put the key-value pair
		err := store.Put(key, value)
		if err != nil {
			rt.Fatalf("Put failed: %v", err)
		}

		// Verify the key exists
		_, err = store.Get(key)
		if err != nil {
			rt.Fatalf("Get after Put failed: %v", err)
		}

		// Delete the key
		err = store.Delete(key)
		if err != nil {
			rt.Fatalf("Delete failed: %v", err)
		}

		// Verify the key no longer exists
		_, err = store.Get(key)
		if err != ErrKeyNotFound {
			rt.Fatalf("Get after Delete should return ErrKeyNotFound, got: %v", err)
		}
	})
}

// TestProperty_TimeTravelGetCorrectness tests Property 11: Time-Travel Get Correctness
// **Feature: versioned-kv-store, Property 11: Time-Travel Get Correctness**
// **Validates: Requirements 6.1**
//
// For any key-value pair that existed at commit C, GetAt(key, C) SHALL return the value
// as it was at commit C, regardless of subsequent modifications.
func TestProperty_TimeTravelGetCorrectness(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a non-empty key and two different values
		key := genNonEmptyBytes(rt, "key")
		value1 := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(rt, "value1")
		value2 := rapid.SliceOfN(rapid.Byte(), 1, 100).Draw(rt, "value2")

		// Put the first value and commit
		err := store.Put(key, value1)
		if err != nil {
			rt.Fatalf("Put value1 failed: %v", err)
		}

		commitHash1, err := store.Commit("first commit")
		if err != nil {
			rt.Fatalf("Commit 1 failed: %v", err)
		}

		// Modify the value and commit again
		err = store.Put(key, value2)
		if err != nil {
			rt.Fatalf("Put value2 failed: %v", err)
		}

		_, err = store.Commit("second commit")
		if err != nil {
			rt.Fatalf("Commit 2 failed: %v", err)
		}

		// Time-travel to the first commit and verify we get the original value
		retrieved, err := store.GetAt(key, commitHash1)
		if err != nil {
			rt.Fatalf("GetAt failed: %v", err)
		}

		// Verify the retrieved value matches value1, not value2
		if len(retrieved) != len(value1) {
			rt.Fatalf("Value length mismatch: got %d, want %d", len(retrieved), len(value1))
		}
		for i := range value1 {
			if retrieved[i] != value1[i] {
				rt.Fatalf("Value mismatch at byte %d: got %d, want %d", i, retrieved[i], value1[i])
			}
		}
	})
}

// TestProperty_CheckoutRestoresState tests Property 12: Checkout Restores State
// **Feature: versioned-kv-store, Property 12: Checkout Restores State**
// **Validates: Requirements 6.4**
//
// For any commit C, after Checkout(C), the working state SHALL match the state at commit C
// (all keys and values identical).
func TestProperty_CheckoutRestoresState(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	rapid.Check(t, func(rt *rapid.T) {
		// Generate multiple key-value pairs for the first state
		numPairs := rapid.IntRange(1, 10).Draw(rt, "numPairs")
		originalPairs := make(map[string][]byte)

		for i := 0; i < numPairs; i++ {
			key := genNonEmptyBytes(rt, "key")
			value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")
			originalPairs[string(key)] = value
			err := store.Put(key, value)
			if err != nil {
				rt.Fatalf("Put failed: %v", err)
			}
		}

		// Commit the first state
		commitHash1, err := store.Commit("first commit")
		if err != nil {
			rt.Fatalf("Commit 1 failed: %v", err)
		}

		// Generate a new key that is guaranteed to be different from all original keys
		// by using a prefix that won't collide
		newKey := append([]byte("__new__"), genNonEmptyBytes(rt, "newKey")...)
		newValue := rapid.SliceOf(rapid.Byte()).Draw(rt, "newValue")
		err = store.Put(newKey, newValue)
		if err != nil {
			rt.Fatalf("Put new key failed: %v", err)
		}

		// Commit the modified state
		_, err = store.Commit("second commit")
		if err != nil {
			rt.Fatalf("Commit 2 failed: %v", err)
		}

		// Checkout back to the first commit
		err = store.Checkout(commitHash1)
		if err != nil {
			rt.Fatalf("Checkout failed: %v", err)
		}

		// Verify all original keys exist with their original values
		for keyStr, expectedValue := range originalPairs {
			key := []byte(keyStr)
			retrieved, err := store.Get(key)
			if err != nil {
				rt.Fatalf("Get key %q after checkout failed: %v", keyStr, err)
			}
			if len(retrieved) != len(expectedValue) {
				rt.Fatalf("Value length mismatch for key %q: got %d, want %d", keyStr, len(retrieved), len(expectedValue))
			}
			for i := range expectedValue {
				if retrieved[i] != expectedValue[i] {
					rt.Fatalf("Value mismatch for key %q at byte %d", keyStr, i)
				}
			}
		}

		// Verify the new key added after commit1 does NOT exist
		_, err = store.Get(newKey)
		if err != ErrKeyNotFound {
			rt.Fatalf("New key should not exist after checkout to earlier commit, got err: %v", err)
		}
	})
}

// TestStore_DiffOperations tests the Diff operation
// This is a unit test to verify Diff works correctly through the Store API
func TestStore_DiffOperations(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	// Create initial state with some keys
	store.Put([]byte("key1"), []byte("value1"))
	store.Put([]byte("key2"), []byte("value2"))
	store.Put([]byte("key3"), []byte("value3"))

	commitHash1, err := store.Commit("initial commit")
	if err != nil {
		t.Fatalf("Commit 1 failed: %v", err)
	}

	// Modify state: add key4, modify key2, delete key3
	store.Put([]byte("key4"), []byte("value4"))
	store.Put([]byte("key2"), []byte("modified_value2"))
	store.Delete([]byte("key3"))

	commitHash2, err := store.Commit("modified commit")
	if err != nil {
		t.Fatalf("Commit 2 failed: %v", err)
	}

	// Get diff between commits
	diff, err := store.Diff(commitHash1, commitHash2)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	// Verify added keys
	if len(diff.Added) != 1 {
		t.Fatalf("Expected 1 added key, got %d", len(diff.Added))
	}
	if string(diff.Added[0].Key) != "key4" {
		t.Fatalf("Expected added key 'key4', got '%s'", string(diff.Added[0].Key))
	}

	// Verify modified keys
	if len(diff.Modified) != 1 {
		t.Fatalf("Expected 1 modified key, got %d", len(diff.Modified))
	}
	if string(diff.Modified[0].Key) != "key2" {
		t.Fatalf("Expected modified key 'key2', got '%s'", string(diff.Modified[0].Key))
	}

	// Verify deleted keys
	if len(diff.Deleted) != 1 {
		t.Fatalf("Expected 1 deleted key, got %d", len(diff.Deleted))
	}
	if string(diff.Deleted[0]) != "key3" {
		t.Fatalf("Expected deleted key 'key3', got '%s'", string(diff.Deleted[0]))
	}
}

// TestStore_LogOperations tests the Log operation
// This is a unit test to verify Log returns commit history correctly
func TestStore_LogOperations(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	// Initially, log should be empty
	log, err := store.Log()
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("Expected empty log, got %d commits", len(log))
	}

	// Create first commit
	store.Put([]byte("key1"), []byte("value1"))
	_, err = store.Commit("first commit")
	if err != nil {
		t.Fatalf("Commit 1 failed: %v", err)
	}

	// Create second commit
	store.Put([]byte("key2"), []byte("value2"))
	_, err = store.Commit("second commit")
	if err != nil {
		t.Fatalf("Commit 2 failed: %v", err)
	}

	// Create third commit
	store.Put([]byte("key3"), []byte("value3"))
	_, err = store.Commit("third commit")
	if err != nil {
		t.Fatalf("Commit 3 failed: %v", err)
	}

	// Get log
	log, err = store.Log()
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify we have 3 commits
	if len(log) != 3 {
		t.Fatalf("Expected 3 commits in log, got %d", len(log))
	}

	// Verify commits are in reverse chronological order (newest first)
	if log[0].Message != "third commit" {
		t.Fatalf("Expected first log entry to be 'third commit', got '%s'", log[0].Message)
	}
	if log[1].Message != "second commit" {
		t.Fatalf("Expected second log entry to be 'second commit', got '%s'", log[1].Message)
	}
	if log[2].Message != "first commit" {
		t.Fatalf("Expected third log entry to be 'first commit', got '%s'", log[2].Message)
	}

	// Verify parent chain integrity
	if log[2].Parent != ZeroHash {
		t.Fatalf("First commit should have ZeroHash parent")
	}
}

// TestStore_DiffIdenticalCommits tests that diffing identical commits returns empty diff
func TestStore_DiffIdenticalCommits(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	store.Put([]byte("key1"), []byte("value1"))
	commitHash, err := store.Commit("commit")
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Diff a commit with itself should return empty diff
	diff, err := store.Diff(commitHash, commitHash)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 0 || len(diff.Modified) != 0 || len(diff.Deleted) != 0 {
		t.Fatalf("Expected empty diff for identical commits, got added=%d, modified=%d, deleted=%d",
			len(diff.Added), len(diff.Modified), len(diff.Deleted))
	}
}

// TestProperty_PersistenceAcrossRestarts tests Property 16: Persistence Across Restarts
// **Feature: versioned-kv-store, Property 16: Persistence Across Restarts**
// **Validates: Requirements 9.2**
//
// For any store with committed data, closing and reopening the store SHALL restore
// the HEAD commit and all accessible history.
func TestProperty_PersistenceAcrossRestarts(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a temporary directory for the store
		tmpDir, err := os.MkdirTemp("", "persistence-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Generate random key-value pairs
		numPairs := rapid.IntRange(1, 10).Draw(rt, "numPairs")
		pairs := make(map[string][]byte)
		for i := 0; i < numPairs; i++ {
			key := genNonEmptyBytes(rt, "key")
			value := rapid.SliceOf(rapid.Byte()).Draw(rt, "value")
			pairs[string(key)] = value
		}

		// Create store, add data, and commit
		store1, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create store: %v", err)
		}

		for keyStr, value := range pairs {
			err := store1.Put([]byte(keyStr), value)
			if err != nil {
				rt.Fatalf("Put failed: %v", err)
			}
		}

		commitHash, err := store1.Commit("test commit")
		if err != nil {
			rt.Fatalf("Commit failed: %v", err)
		}

		// Get the log before closing
		logBefore, err := store1.Log()
		if err != nil {
			rt.Fatalf("Log failed: %v", err)
		}

		// Close the store
		err = store1.Close()
		if err != nil {
			rt.Fatalf("Close failed: %v", err)
		}

		// Reopen the store from the same directory
		store2, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to reopen store: %v", err)
		}
		defer store2.Close()

		// Verify HEAD is restored
		if store2.Head() != commitHash {
			rt.Fatalf("HEAD not restored: got %s, want %s", store2.Head().String(), commitHash.String())
		}

		// Verify all key-value pairs are accessible
		for keyStr, expectedValue := range pairs {
			retrieved, err := store2.Get([]byte(keyStr))
			if err != nil {
				rt.Fatalf("Get key %q after restart failed: %v", keyStr, err)
			}
			if len(retrieved) != len(expectedValue) {
				rt.Fatalf("Value length mismatch for key %q: got %d, want %d", keyStr, len(retrieved), len(expectedValue))
			}
			for i := range expectedValue {
				if retrieved[i] != expectedValue[i] {
					rt.Fatalf("Value mismatch for key %q at byte %d", keyStr, i)
				}
			}
		}

		// Verify commit history is accessible
		logAfter, err := store2.Log()
		if err != nil {
			rt.Fatalf("Log after restart failed: %v", err)
		}

		if len(logAfter) != len(logBefore) {
			rt.Fatalf("Log length mismatch: got %d, want %d", len(logAfter), len(logBefore))
		}

		// Verify each commit in the log matches
		for i := range logBefore {
			if logAfter[i].RootHash != logBefore[i].RootHash {
				rt.Fatalf("Commit %d RootHash mismatch", i)
			}
			if logAfter[i].Message != logBefore[i].Message {
				rt.Fatalf("Commit %d Message mismatch", i)
			}
			if logAfter[i].Parent != logBefore[i].Parent {
				rt.Fatalf("Commit %d Parent mismatch", i)
			}
			if logAfter[i].Timestamp != logBefore[i].Timestamp {
				rt.Fatalf("Commit %d Timestamp mismatch", i)
			}
		}
	})
}

// createTestStoreWithDir creates a Store with a specific directory for testing
func createTestStoreWithDir(t *testing.T) (*Store, string, func()) {
	tmpDir, err := os.MkdirTemp("", "store-branch-test-*")
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, tmpDir, cleanup
}

// TestProperty_SwitchBranchUpdatesWorkingState tests Property 5: Switch Branch Updates Working State
// **Feature: branching, Property 5: Switch Branch Updates Working State**
// **Validates: Requirements 3.1, 3.3**
//
// For any branch with committed data, after SwitchBranch(), the working state SHALL match
// the data at that branch's commit.
func TestProperty_SwitchBranchUpdatesWorkingState(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh Store for each test
		tmpDir, err := os.MkdirTemp("", "switch-branch-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create Store: %v", err)
		}
		defer store.Close()

		// Generate key-value pairs for branch1 (main)
		numPairs1 := rapid.IntRange(1, 5).Draw(rt, "numPairs1")
		branch1Data := make(map[string][]byte)
		for i := 0; i < numPairs1; i++ {
			key := genNonEmptyBytes(rt, "key1")
			value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(rt, "value1")
			branch1Data[string(key)] = value
			if err := store.Put(key, value); err != nil {
				rt.Fatalf("Put failed: %v", err)
			}
		}

		// Commit on main branch
		_, err = store.Commit("commit on main")
		if err != nil {
			rt.Fatalf("Commit on main failed: %v", err)
		}

		// Create a new branch
		err = store.CreateBranch("feature")
		if err != nil {
			rt.Fatalf("CreateBranch failed: %v", err)
		}

		// Switch to feature branch
		err = store.SwitchBranch("feature")
		if err != nil {
			rt.Fatalf("SwitchBranch to feature failed: %v", err)
		}

		// Add different data on feature branch
		numPairs2 := rapid.IntRange(1, 5).Draw(rt, "numPairs2")
		branch2Data := make(map[string][]byte)
		// Copy existing data first
		for k, v := range branch1Data {
			branch2Data[k] = v
		}
		// Add new unique keys
		for i := 0; i < numPairs2; i++ {
			key := append([]byte("feature_"), genNonEmptyBytes(rt, "key2")...)
			value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(rt, "value2")
			branch2Data[string(key)] = value
			if err := store.Put(key, value); err != nil {
				rt.Fatalf("Put on feature failed: %v", err)
			}
		}

		// Commit on feature branch
		_, err = store.Commit("commit on feature")
		if err != nil {
			rt.Fatalf("Commit on feature failed: %v", err)
		}

		// Switch back to main
		err = store.SwitchBranch("main")
		if err != nil {
			rt.Fatalf("SwitchBranch to main failed: %v", err)
		}

		// Verify working state matches main branch data
		for keyStr, expectedValue := range branch1Data {
			key := []byte(keyStr)
			retrieved, err := store.Get(key)
			if err != nil {
				rt.Fatalf("Get key %q after switch to main failed: %v", keyStr, err)
			}
			if len(retrieved) != len(expectedValue) {
				rt.Fatalf("Value length mismatch for key %q: got %d, want %d", keyStr, len(retrieved), len(expectedValue))
			}
			for i := range expectedValue {
				if retrieved[i] != expectedValue[i] {
					rt.Fatalf("Value mismatch for key %q at byte %d", keyStr, i)
				}
			}
		}

		// Verify feature-only keys don't exist on main
		for keyStr := range branch2Data {
			if _, exists := branch1Data[keyStr]; !exists {
				key := []byte(keyStr)
				_, err := store.Get(key)
				if err != ErrKeyNotFound {
					rt.Fatalf("Feature-only key %q should not exist on main, got err: %v", keyStr, err)
				}
			}
		}

		// Switch to feature and verify its data
		err = store.SwitchBranch("feature")
		if err != nil {
			rt.Fatalf("SwitchBranch to feature (second time) failed: %v", err)
		}

		for keyStr, expectedValue := range branch2Data {
			key := []byte(keyStr)
			retrieved, err := store.Get(key)
			if err != nil {
				rt.Fatalf("Get key %q after switch to feature failed: %v", keyStr, err)
			}
			if len(retrieved) != len(expectedValue) {
				rt.Fatalf("Value length mismatch for key %q on feature: got %d, want %d", keyStr, len(retrieved), len(expectedValue))
			}
			for i := range expectedValue {
				if retrieved[i] != expectedValue[i] {
					rt.Fatalf("Value mismatch for key %q on feature at byte %d", keyStr, i)
				}
			}
		}
	})
}

// TestProperty_CurrentBranchTracking tests Property 6: Current Branch Tracking
// **Feature: branching, Property 6: Current Branch Tracking**
// **Validates: Requirements 2.4**
//
// For any branch switch operation, CurrentBranch() SHALL return the name of the branch
// that was switched to.
func TestProperty_CurrentBranchTracking(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh Store for each test
		tmpDir, err := os.MkdirTemp("", "current-branch-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create Store: %v", err)
		}
		defer store.Close()

		// Initially should be on main branch
		branchName, isDetached, err := store.CurrentBranch()
		if err != nil {
			rt.Fatalf("CurrentBranch failed: %v", err)
		}
		if branchName != "main" {
			rt.Fatalf("Expected initial branch to be 'main', got %q", branchName)
		}
		if isDetached {
			rt.Fatalf("Expected HEAD to be attached initially")
		}

		// Make a commit so we can create branches
		store.Put([]byte("key"), []byte("value"))
		_, err = store.Commit("initial commit")
		if err != nil {
			rt.Fatalf("Initial commit failed: %v", err)
		}

		// Generate a valid branch name
		branchSuffix := rapid.StringMatching(`[a-z][a-z0-9]{0,9}`).Draw(rt, "branch_suffix")
		if branchSuffix == "" {
			branchSuffix = "test"
		}
		newBranchName := "feature-" + branchSuffix

		// Create and switch to new branch
		err = store.CreateBranch(newBranchName)
		if err != nil {
			rt.Fatalf("CreateBranch(%q) failed: %v", newBranchName, err)
		}

		err = store.SwitchBranch(newBranchName)
		if err != nil {
			rt.Fatalf("SwitchBranch(%q) failed: %v", newBranchName, err)
		}

		// Verify CurrentBranch returns the new branch
		branchName, isDetached, err = store.CurrentBranch()
		if err != nil {
			rt.Fatalf("CurrentBranch after switch failed: %v", err)
		}
		if branchName != newBranchName {
			rt.Fatalf("Expected current branch to be %q, got %q", newBranchName, branchName)
		}
		if isDetached {
			rt.Fatalf("Expected HEAD to be attached after SwitchBranch")
		}

		// Switch back to main
		err = store.SwitchBranch("main")
		if err != nil {
			rt.Fatalf("SwitchBranch to main failed: %v", err)
		}

		// Verify CurrentBranch returns main
		branchName, isDetached, err = store.CurrentBranch()
		if err != nil {
			rt.Fatalf("CurrentBranch after switch to main failed: %v", err)
		}
		if branchName != "main" {
			rt.Fatalf("Expected current branch to be 'main', got %q", branchName)
		}
		if isDetached {
			rt.Fatalf("Expected HEAD to be attached after SwitchBranch to main")
		}
	})
}

// TestProperty_CommitAdvancesBranch tests Property 7: Commit Advances Branch
// **Feature: branching, Property 7: Commit Advances Branch**
// **Validates: Requirements 5.1**
//
// For any commit made while HEAD points to a branch, that branch SHALL be updated
// to point to the new commit.
func TestProperty_CommitAdvancesBranch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh Store for each test
		tmpDir, err := os.MkdirTemp("", "commit-advances-branch-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create Store: %v", err)
		}
		defer store.Close()

		// Verify we're on main branch
		branchName, isDetached, err := store.CurrentBranch()
		if err != nil {
			rt.Fatalf("CurrentBranch failed: %v", err)
		}
		if branchName != "main" || isDetached {
			rt.Fatalf("Expected to be on main branch, got %q (detached=%v)", branchName, isDetached)
		}

		// Generate and add some data
		numPairs := rapid.IntRange(1, 5).Draw(rt, "numPairs")
		for i := 0; i < numPairs; i++ {
			key := genNonEmptyBytes(rt, "key")
			value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(rt, "value")
			if err := store.Put(key, value); err != nil {
				rt.Fatalf("Put failed: %v", err)
			}
		}

		// Commit
		commitHash, err := store.Commit("test commit")
		if err != nil {
			rt.Fatalf("Commit failed: %v", err)
		}

		// Verify HEAD points to the new commit
		if store.Head() != commitHash {
			rt.Fatalf("HEAD should point to new commit: got %s, want %s", store.Head().String(), commitHash.String())
		}

		// Verify the branch was updated to point to the new commit
		// We need to access the branch manager directly through the store
		// Since we're on main, switching away and back should give us the same commit
		err = store.CreateBranch("temp")
		if err != nil {
			rt.Fatalf("CreateBranch temp failed: %v", err)
		}

		err = store.SwitchBranch("temp")
		if err != nil {
			rt.Fatalf("SwitchBranch to temp failed: %v", err)
		}

		err = store.SwitchBranch("main")
		if err != nil {
			rt.Fatalf("SwitchBranch back to main failed: %v", err)
		}

		// After switching back, HEAD should still point to the commit we made
		if store.Head() != commitHash {
			rt.Fatalf("After switch back, HEAD should point to commit: got %s, want %s", store.Head().String(), commitHash.String())
		}
	})
}

// TestProperty_DetachedCommitPreservesBranches tests Property 8: Detached Commit Preserves Branches
// **Feature: branching, Property 8: Detached Commit Preserves Branches**
// **Validates: Requirements 5.2**
//
// For any commit made while HEAD is detached, all existing branches SHALL remain unchanged.
func TestProperty_DetachedCommitPreservesBranches(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a fresh Store for each test
		tmpDir, err := os.MkdirTemp("", "detached-commit-test-*")
		if err != nil {
			rt.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		store, err := NewStore(tmpDir)
		if err != nil {
			rt.Fatalf("Failed to create Store: %v", err)
		}
		defer store.Close()

		// Make an initial commit on main
		store.Put([]byte("initial"), []byte("data"))
		initialCommit, err := store.Commit("initial commit")
		if err != nil {
			rt.Fatalf("Initial commit failed: %v", err)
		}

		// Create another branch at the same commit
		err = store.CreateBranch("feature")
		if err != nil {
			rt.Fatalf("CreateBranch feature failed: %v", err)
		}

		// Detach HEAD to the initial commit
		err = store.DetachHead(initialCommit)
		if err != nil {
			rt.Fatalf("DetachHead failed: %v", err)
		}

		// Verify we're in detached state
		_, isDetached, err := store.CurrentBranch()
		if err != nil {
			rt.Fatalf("CurrentBranch failed: %v", err)
		}
		if !isDetached {
			rt.Fatalf("Expected HEAD to be detached")
		}

		// Make a commit in detached state
		numPairs := rapid.IntRange(1, 3).Draw(rt, "numPairs")
		for i := 0; i < numPairs; i++ {
			key := append([]byte("detached_"), genNonEmptyBytes(rt, "key")...)
			value := rapid.SliceOfN(rapid.Byte(), 1, 50).Draw(rt, "value")
			if err := store.Put(key, value); err != nil {
				rt.Fatalf("Put in detached state failed: %v", err)
			}
		}

		detachedCommit, err := store.Commit("detached commit")
		if err != nil {
			rt.Fatalf("Commit in detached state failed: %v", err)
		}

		// Verify HEAD moved to the new commit
		if store.Head() != detachedCommit {
			rt.Fatalf("HEAD should point to detached commit")
		}

		// Verify main branch still points to initial commit
		err = store.SwitchBranch("main")
		if err != nil {
			rt.Fatalf("SwitchBranch to main failed: %v", err)
		}

		if store.Head() != initialCommit {
			rt.Fatalf("main branch should still point to initial commit: got %s, want %s",
				store.Head().String(), initialCommit.String())
		}

		// Verify feature branch still points to initial commit
		err = store.SwitchBranch("feature")
		if err != nil {
			rt.Fatalf("SwitchBranch to feature failed: %v", err)
		}

		if store.Head() != initialCommit {
			rt.Fatalf("feature branch should still point to initial commit: got %s, want %s",
				store.Head().String(), initialCommit.String())
		}
	})
}
