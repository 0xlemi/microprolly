package cas

import (
	"os"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_CASWriteReadRoundTrip tests Property 7: CAS Write-Read Round-Trip
// **Feature: versioned-kv-store, Property 7: CAS Write-Read Round-Trip**
// **Validates: Requirements 4.1, 4.2, 4.3, 4.5**
//
// For any byte sequence, Write(data) followed by Read(hash) SHALL return the original data,
// and writing the same data twice SHALL return the same hash.
func TestProperty_CASWriteReadRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create a temporary directory for this test iteration
		tmpDir, err := os.MkdirTemp("", "cas-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Create CAS instance
		cas, err := NewFileCAS(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		defer cas.Close()

		// Generate random data
		data := rapid.SliceOf(rapid.Byte()).Draw(t, "data")

		// Write data and get hash
		hash1, err := cas.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Read data back
		readData, err := cas.Read(hash1)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		// Verify round-trip: data should be identical
		if len(data) != len(readData) {
			t.Fatalf("Round-trip failed: length mismatch, got %d, want %d", len(readData), len(data))
		}
		for i := range data {
			if data[i] != readData[i] {
				t.Fatalf("Round-trip failed: byte mismatch at index %d", i)
			}
		}

		// Write same data again - should return same hash (deduplication)
		hash2, err := cas.Write(data)
		if err != nil {
			t.Fatalf("Second Write failed: %v", err)
		}

		// Verify idempotence: same data produces same hash
		if hash1 != hash2 {
			t.Fatalf("Idempotence failed: hash1=%s, hash2=%s", hash1.String(), hash2.String())
		}

		// Verify Exists returns true for written hash
		if !cas.Exists(hash1) {
			t.Fatal("Exists returned false for written hash")
		}
	})
}
