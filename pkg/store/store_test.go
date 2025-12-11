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
