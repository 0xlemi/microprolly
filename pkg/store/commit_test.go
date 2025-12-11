package store

import (
	"os"
	"testing"
	"time"

	"microprolly/pkg/cas"
	"microprolly/pkg/types"

	"pgregory.net/rapid"
)

// genHash generates a random Hash for testing
func genHash(t *rapid.T, name string) types.Hash {
	var h types.Hash
	hashBytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, name)
	copy(h[:], hashBytes)
	return h
}

// TestProperty_CommitSerializationRoundTrip tests Property 10: Commit Serialization Round-Trip
// **Feature: versioned-kv-store, Property 10: Commit Serialization Round-Trip**
// **Validates: Requirements 5.5**
//
// For any commit object, serializing to JSON then deserializing SHALL produce an equivalent commit object.
func TestProperty_CommitSerializationRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random commit
		commit := &types.Commit{
			RootHash:  genHash(t, "root_hash"),
			Message:   rapid.String().Draw(t, "message"),
			Parent:    genHash(t, "parent"),
			Timestamp: rapid.Int64().Draw(t, "timestamp"),
		}

		// Serialize to JSON
		data, err := MarshalCommit(commit)
		if err != nil {
			t.Fatalf("MarshalCommit failed: %v", err)
		}

		// Deserialize back
		restored, err := UnmarshalCommit(data)
		if err != nil {
			t.Fatalf("UnmarshalCommit failed: %v", err)
		}

		// Verify round-trip: all fields should be identical
		if commit.RootHash != restored.RootHash {
			t.Fatalf("RootHash mismatch: got %s, want %s", restored.RootHash.String(), commit.RootHash.String())
		}
		if commit.Message != restored.Message {
			t.Fatalf("Message mismatch: got %q, want %q", restored.Message, commit.Message)
		}
		if commit.Parent != restored.Parent {
			t.Fatalf("Parent mismatch: got %s, want %s", restored.Parent.String(), commit.Parent.String())
		}
		if commit.Timestamp != restored.Timestamp {
			t.Fatalf("Timestamp mismatch: got %d, want %d", restored.Timestamp, commit.Timestamp)
		}
	})
}

// TestProperty_CommitStructureCompleteness tests Property 8: Commit Structure Completeness
// **Feature: versioned-kv-store, Property 8: Commit Structure Completeness**
// **Validates: Requirements 5.1, 5.2**
//
// For any commit created via CreateCommit, the commit object SHALL contain a valid root hash,
// the provided message, a timestamp, and the parent commit hash.
func TestProperty_CommitStructureCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create a temporary directory for CAS
		tmpDir, err := os.MkdirTemp("", "commit-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Create CAS and CommitManager
		casStore, err := cas.NewFileCAS(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		defer casStore.Close()

		cm := NewCommitManager(casStore)

		// Generate random inputs
		rootHash := genHash(t, "root_hash")
		message := rapid.String().Draw(t, "message")
		parent := genHash(t, "parent")

		// Record time before creating commit
		beforeTime := time.Now().Unix()

		// Create commit
		commit, commitHash, err := cm.CreateCommit(rootHash, message, parent)
		if err != nil {
			t.Fatalf("CreateCommit failed: %v", err)
		}

		// Record time after creating commit
		afterTime := time.Now().Unix()

		// Verify commit structure completeness

		// 1. Root hash should match the provided root hash
		if commit.RootHash != rootHash {
			t.Fatalf("RootHash mismatch: got %s, want %s", commit.RootHash.String(), rootHash.String())
		}

		// 2. Message should match the provided message
		if commit.Message != message {
			t.Fatalf("Message mismatch: got %q, want %q", commit.Message, message)
		}

		// 3. Parent should match the provided parent
		if commit.Parent != parent {
			t.Fatalf("Parent mismatch: got %s, want %s", commit.Parent.String(), parent.String())
		}

		// 4. Timestamp should be within the time window of creation
		if commit.Timestamp < beforeTime || commit.Timestamp > afterTime {
			t.Fatalf("Timestamp out of range: got %d, expected between %d and %d", commit.Timestamp, beforeTime, afterTime)
		}

		// 5. Commit should be stored in CAS and retrievable
		if !casStore.Exists(commitHash) {
			t.Fatal("Commit hash not found in CAS")
		}

		// 6. Retrieved commit should match the original
		retrieved, err := cm.GetCommit(commitHash)
		if err != nil {
			t.Fatalf("GetCommit failed: %v", err)
		}

		if retrieved.RootHash != commit.RootHash {
			t.Fatalf("Retrieved RootHash mismatch")
		}
		if retrieved.Message != commit.Message {
			t.Fatalf("Retrieved Message mismatch")
		}
		if retrieved.Parent != commit.Parent {
			t.Fatalf("Retrieved Parent mismatch")
		}
		if retrieved.Timestamp != commit.Timestamp {
			t.Fatalf("Retrieved Timestamp mismatch")
		}
	})
}

// TestProperty_CommitHistoryChainIntegrity tests Property 9: Commit History Chain Integrity
// **Feature: versioned-kv-store, Property 9: Commit History Chain Integrity**
// **Validates: Requirements 5.3**
//
// For any sequence of N commits, Log() SHALL return exactly N commits in reverse chronological order,
// with each commit's parent matching the previous commit's hash.
func TestProperty_CommitHistoryChainIntegrity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create a temporary directory for CAS
		tmpDir, err := os.MkdirTemp("", "commit-chain-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Create CAS and CommitManager
		casStore, err := cas.NewFileCAS(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		defer casStore.Close()

		cm := NewCommitManager(casStore)

		// Generate a random number of commits (1 to 10)
		numCommits := rapid.IntRange(1, 10).Draw(t, "numCommits")

		// Create a chain of commits
		var commitHashes []types.Hash
		parentHash := ZeroHash // First commit has no parent

		for i := 0; i < numCommits; i++ {
			rootHash := genHash(t, "root_hash")
			message := rapid.String().Draw(t, "message")

			_, commitHash, err := cm.CreateCommit(rootHash, message, parentHash)
			if err != nil {
				t.Fatalf("CreateCommit %d failed: %v", i, err)
			}

			commitHashes = append(commitHashes, commitHash)
			parentHash = commitHash // Next commit's parent is this commit
		}

		// Get the log starting from the last commit
		lastCommitHash := commitHashes[len(commitHashes)-1]
		log, err := cm.Log(lastCommitHash)
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}

		// Verify: Log should return exactly N commits
		if len(log) != numCommits {
			t.Fatalf("Log returned %d commits, expected %d", len(log), numCommits)
		}

		// Verify: Commits should be in reverse chronological order
		// (newest first, which means the last created commit should be first in log)
		for i := 0; i < len(log)-1; i++ {
			// Each commit's parent should match the hash of the next commit in the log
			// (since log is reverse chronological)
			expectedParentHash := commitHashes[len(commitHashes)-2-i]
			if log[i].Parent != expectedParentHash {
				t.Fatalf("Commit %d parent mismatch: got %s, want %s",
					i, log[i].Parent.String(), expectedParentHash.String())
			}
		}

		// Verify: The last commit in the log (oldest) should have ZeroHash as parent
		if log[len(log)-1].Parent != ZeroHash {
			t.Fatalf("First commit should have ZeroHash parent, got %s", log[len(log)-1].Parent.String())
		}

		// Verify: Chain integrity - each commit's parent matches the previous commit's hash
		for i := 0; i < len(log)-1; i++ {
			// The parent of log[i] should be the hash of log[i+1]
			// We need to verify this by checking that the parent hash stored in log[i]
			// matches the hash we stored for the commit at position i+1
			expectedParent := commitHashes[len(commitHashes)-2-i]
			if log[i].Parent != expectedParent {
				t.Fatalf("Chain integrity broken at position %d", i)
			}
		}
	})
}
