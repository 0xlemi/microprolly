// Package main demonstrates the MicroProlly versioned key-value store.
//
// This example shows:
// - Basic CRUD operations (Put, Get, Delete)
// - Committing changes to create snapshots
// - Time-travel queries to access historical data
// - Diffing between versions
// - Viewing commit history
// - Checkout to restore old state
//
// Run with: go run examples/demo/main.go
package main

import (
	"fmt"
	"log"
	"os"

	"microprolly/pkg/store"
	"microprolly/pkg/types"
)

// ANSI color codes
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
)

func main() {
	// Clean up any previous demo data
	dataDir := "./demo-data"
	os.RemoveAll(dataDir)

	printHeader("MicroProlly Demo")
	fmt.Println()

	// 1. Create a new store
	printStep(1, "Creating store")
	db, err := store.NewStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		db.Close()
		os.RemoveAll(dataDir) // Clean up demo data
	}()
	fmt.Printf("   Store created at: %s%s%s\n", Cyan, dataDir, Reset)
	fmt.Println()

	// 2. Basic Put/Get operations
	printStep(2, "Basic operations")
	users := map[string]string{
		"user:1": "Alice",
		"user:2": "Bob",
		"user:3": "Charlie",
	}
	for k, v := range users {
		db.Put([]byte(k), []byte(v))
	}
	fmt.Printf("   Added 3 users: %sAlice%s, %sBob%s, %sCharlie%s\n",
		Green, Reset, Green, Reset, Green, Reset)

	value, _ := db.Get([]byte("user:1"))
	fmt.Printf("   Get %suser:1%s = %s%s%s\n", Yellow, Reset, Green, string(value), Reset)
	fmt.Println()

	// 3. First commit
	printStep(3, "Creating first commit")
	commit1, err := db.Commit("Initial users")
	if err != nil {
		log.Fatalf("Failed to commit: %v", err)
	}
	fmt.Printf("   Commit 1: %s%s%s %s\"Initial users\"%s\n",
		Yellow, shortHash(commit1), Reset, Dim, Reset)
	fmt.Println()

	// 4. Make some changes
	printStep(4, "Making changes")
	db.Put([]byte("user:1"), []byte("Alice Smith")) // Modify
	db.Put([]byte("user:4"), []byte("Diana"))       // Add
	db.Delete([]byte("user:3"))                     // Delete
	fmt.Printf("   %s~%s Modified %suser:1%s -> %s'Alice Smith'%s\n",
		Yellow, Reset, Cyan, Reset, Green, Reset)
	fmt.Printf("   %s+%s Added    %suser:4%s -> %s'Diana'%s\n",
		Green, Reset, Cyan, Reset, Green, Reset)
	fmt.Printf("   %s-%s Deleted  %suser:3%s %s(Charlie)%s\n",
		Red, Reset, Cyan, Reset, Dim, Reset)
	fmt.Println()

	// 5. Second commit
	printStep(5, "Creating second commit")
	commit2, err := db.Commit("Updated users")
	if err != nil {
		log.Fatalf("Failed to commit: %v", err)
	}
	fmt.Printf("   Commit 2: %s%s%s %s\"Updated users\"%s\n",
		Yellow, shortHash(commit2), Reset, Dim, Reset)
	fmt.Println()

	// 6. Time travel - query old data
	printStep(6, "Time travel queries")
	oldValue, _ := db.GetAt([]byte("user:1"), commit1)
	newValue, _ := db.Get([]byte("user:1"))
	fmt.Printf("   %suser:1%s at commit 1: %s%s%s\n",
		Cyan, Reset, Green, string(oldValue), Reset)
	fmt.Printf("   %suser:1%s at commit 2: %s%s%s\n",
		Cyan, Reset, Green, string(newValue), Reset)

	oldCharlie, err := db.GetAt([]byte("user:3"), commit1)
	if err == nil {
		fmt.Printf("   %suser:3%s at commit 1: %s%s%s %s(still exists!)%s\n",
			Cyan, Reset, Green, string(oldCharlie), Reset, Dim, Reset)
	}
	_, err = db.Get([]byte("user:3"))
	if err == store.ErrKeyNotFound {
		fmt.Printf("   %suser:3%s at commit 2: %s<deleted>%s\n",
			Cyan, Reset, Red, Reset)
	}
	fmt.Println()

	// 7. Diff between commits
	printStep(7, "Diff between commits")
	diff, err := db.Diff(commit1, commit2)
	if err != nil {
		log.Fatalf("Failed to diff: %v", err)
	}

	fmt.Printf("   %sAdded%s (%d):\n", Green, Reset, len(diff.Added))
	for _, kv := range diff.Added {
		fmt.Printf("     %s+%s %s%s%s = %s\n",
			Green, Reset, Cyan, string(kv.Key), Reset, string(kv.Value))
	}

	fmt.Printf("   %sModified%s (%d):\n", Yellow, Reset, len(diff.Modified))
	for _, m := range diff.Modified {
		fmt.Printf("     %s~%s %s%s%s: %s%s%s -> %s%s%s\n",
			Yellow, Reset, Cyan, string(m.Key), Reset,
			Red, string(m.OldValue), Reset,
			Green, string(m.NewValue), Reset)
	}

	fmt.Printf("   %sDeleted%s (%d):\n", Red, Reset, len(diff.Deleted))
	for _, key := range diff.Deleted {
		fmt.Printf("     %s-%s %s%s%s\n", Red, Reset, Cyan, string(key), Reset)
	}
	fmt.Println()

	// 8. View commit history
	printStep(8, "Commit history")
	commits, err := db.Log()
	if err != nil {
		log.Fatalf("Failed to get log: %v", err)
	}
	for i, c := range commits {
		hash := hashCommit(c)
		fmt.Printf("   %s[%d]%s %s%s%s %s%s%s\n",
			Dim, i+1, Reset, Yellow, shortHash(hash), Reset, White, c.Message, Reset)
	}
	fmt.Println()

	// 9. Checkout old version
	printStep(9, "Checkout to commit 1")
	err = db.Checkout(commit1)
	if err != nil {
		log.Fatalf("Failed to checkout: %v", err)
	}
	fmt.Printf("   Restored working state to commit %s%s%s\n", Yellow, shortHash(commit1), Reset)
	fmt.Println()

	// Verify the state
	fmt.Printf("   %sVerifying restored state:%s\n", Bold, Reset)
	value, _ = db.Get([]byte("user:1"))
	fmt.Printf("   %suser:1%s = %s%s%s %s(original value restored)%s\n",
		Cyan, Reset, Green, string(value), Reset, Dim, Reset)
	value, _ = db.Get([]byte("user:3"))
	fmt.Printf("   %suser:3%s = %s%s%s %s(Charlie is back)%s\n",
		Cyan, Reset, Green, string(value), Reset, Dim, Reset)
	_, err = db.Get([]byte("user:4"))
	if err == store.ErrKeyNotFound {
		fmt.Printf("   %suser:4%s = %s<not found>%s %s(Diana doesn't exist yet)%s\n",
			Cyan, Reset, Red, Reset, Dim, Reset)
	}
	fmt.Println()

	// 10. Make changes from old state (creates new branch point)
	printStep(10, "Making changes from old state")
	db.Put([]byte("user:5"), []byte("Eve"))
	commit3, _ := db.Commit("Added Eve from old state")
	fmt.Printf("   Commit 3: %s%s%s %s\"Added Eve from old state\"%s\n",
		Yellow, shortHash(commit3), Reset, Dim, Reset)
	fmt.Printf("   %s+%s Added %suser:5%s -> %s'Eve'%s %s(branching from commit 1)%s\n",
		Green, Reset, Cyan, Reset, Green, Reset, Dim, Reset)
	fmt.Println()

	// Final state
	printHeader("Final State")
	fmt.Printf("HEAD: %s%s%s\n", Yellow, shortHash(db.Head()), Reset)
	fmt.Println()
	fmt.Printf("%sKeys in working state:%s\n", Bold, Reset)
	for _, key := range []string{"user:1", "user:2", "user:3", "user:4", "user:5"} {
		val, err := db.Get([]byte(key))
		if err == nil {
			fmt.Printf("  %s%s%s = %s%s%s\n", Cyan, key, Reset, Green, string(val), Reset)
		}
	}
	fmt.Println()
	fmt.Printf("%sDemo complete!%s\n", Bold, Reset)
}

// printHeader prints a formatted header
func printHeader(title string) {
	line := "========================================"
	fmt.Printf("%s%s%s\n", Magenta, line, Reset)
	fmt.Printf("%s%s  %s%s\n", Bold, Magenta, title, Reset)
	fmt.Printf("%s%s%s\n", Magenta, line, Reset)
}

// printStep prints a formatted step header
func printStep(num int, title string) {
	fmt.Printf("%s%d. %s%s\n", Bold, num, title, Reset)
}

// shortHash returns first 8 characters of a hash for display
func shortHash(h types.Hash) string {
	return h.String()[:8]
}

// hashCommit computes the hash of a commit (for display purposes)
func hashCommit(c *types.Commit) types.Hash {
	return c.RootHash
}
