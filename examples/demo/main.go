// Package main demonstrates the MicroProlly versioned key-value store.
//
// This example shows:
// - Basic CRUD operations (Put, Get, Delete)
// - Committing changes to create snapshots
// - Time-travel queries to access historical data
// - Diffing between versions
// - Viewing commit history
// - Checkout to restore old state
// - Branching: create, switch, list, and delete branches
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

	// Show default branch
	branchName, _, _ := db.CurrentBranch()
	fmt.Printf("   Default branch: %s%s%s\n", Green, branchName, Reset)
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

	// ========================================
	// BRANCHING DEMO
	// ========================================
	printHeader("Branching Demo")
	fmt.Println()

	// 9. Create a feature branch
	printStep(9, "Create a feature branch")
	err = db.CreateBranch("feature-x")
	if err != nil {
		log.Fatalf("Failed to create branch: %v", err)
	}
	fmt.Printf("   Created branch: %s%s%s\n", Green, "feature-x", Reset)

	// List all branches
	branches, _ := db.ListBranches()
	fmt.Printf("   All branches: ")
	for i, b := range branches {
		if i > 0 {
			fmt.Printf(", ")
		}
		currentBranch, _, _ := db.CurrentBranch()
		if b == currentBranch {
			fmt.Printf("%s%s*%s", Green, b, Reset)
		} else {
			fmt.Printf("%s", b)
		}
	}
	fmt.Println()
	fmt.Println()

	// 10. Switch to feature branch
	printStep(10, "Switch to feature branch")
	err = db.SwitchBranch("feature-x")
	if err != nil {
		log.Fatalf("Failed to switch branch: %v", err)
	}
	branchName, isDetached, _ := db.CurrentBranch()
	fmt.Printf("   Switched to: %s%s%s", Green, branchName, Reset)
	if isDetached {
		fmt.Printf(" %s(detached)%s", Yellow, Reset)
	}
	fmt.Println()
	fmt.Println()

	// 11. Make changes on feature branch
	printStep(11, "Make changes on feature branch")
	db.Put([]byte("feature:1"), []byte("New Feature"))
	db.Put([]byte("user:5"), []byte("Eve"))
	fmt.Printf("   %s+%s Added %sfeature:1%s -> %s'New Feature'%s\n",
		Green, Reset, Cyan, Reset, Green, Reset)
	fmt.Printf("   %s+%s Added %suser:5%s -> %s'Eve'%s\n",
		Green, Reset, Cyan, Reset, Green, Reset)

	featureCommit, _ := db.Commit("Add feature and Eve")
	fmt.Printf("   Commit: %s%s%s %s\"Add feature and Eve\"%s\n",
		Yellow, shortHash(featureCommit), Reset, Dim, Reset)
	fmt.Println()

	// 12. Switch back to main
	printStep(12, "Switch back to main")
	err = db.SwitchBranch("main")
	if err != nil {
		log.Fatalf("Failed to switch branch: %v", err)
	}
	branchName, _, _ = db.CurrentBranch()
	fmt.Printf("   Switched to: %s%s%s\n", Green, branchName, Reset)

	// Verify feature changes are not visible on main
	_, err = db.Get([]byte("feature:1"))
	if err == store.ErrKeyNotFound {
		fmt.Printf("   %sfeature:1%s = %s<not found>%s %s(only on feature-x)%s\n",
			Cyan, Reset, Red, Reset, Dim, Reset)
	}
	_, err = db.Get([]byte("user:5"))
	if err == store.ErrKeyNotFound {
		fmt.Printf("   %suser:5%s = %s<not found>%s %s(only on feature-x)%s\n",
			Cyan, Reset, Red, Reset, Dim, Reset)
	}
	fmt.Println()

	// 13. Make independent changes on main
	printStep(13, "Make independent changes on main")
	db.Put([]byte("config:version"), []byte("2.0"))
	mainCommit, _ := db.Commit("Bump version")
	fmt.Printf("   %s+%s Added %sconfig:version%s -> %s'2.0'%s\n",
		Green, Reset, Cyan, Reset, Green, Reset)
	fmt.Printf("   Commit: %s%s%s %s\"Bump version\"%s\n",
		Yellow, shortHash(mainCommit), Reset, Dim, Reset)
	fmt.Println()

	// 14. Show diverged branches
	printStep(14, "Show diverged branches")
	fmt.Printf("   %smain%s history:\n", Green, Reset)
	mainCommits, _ := db.Log()
	for i, c := range mainCommits {
		if i >= 3 {
			fmt.Printf("     %s...%s\n", Dim, Reset)
			break
		}
		fmt.Printf("     %s%s%s %s\n", Yellow, shortHash(hashCommit(c)), Reset, c.Message)
	}

	db.SwitchBranch("feature-x")
	fmt.Printf("   %sfeature-x%s history:\n", Green, Reset)
	featureCommits, _ := db.Log()
	for i, c := range featureCommits {
		if i >= 3 {
			fmt.Printf("     %s...%s\n", Dim, Reset)
			break
		}
		fmt.Printf("     %s%s%s %s\n", Yellow, shortHash(hashCommit(c)), Reset, c.Message)
	}
	fmt.Println()

	// 15. Detached HEAD state
	printStep(15, "Detached HEAD state")
	db.SwitchBranch("main")
	err = db.DetachHead(commit1)
	if err != nil {
		log.Fatalf("Failed to detach head: %v", err)
	}
	branchName, isDetached, _ = db.CurrentBranch()
	fmt.Printf("   HEAD detached at: %s%s%s\n", Yellow, shortHash(commit1), Reset)
	fmt.Printf("   IsDetached: %s%v%s\n", Yellow, isDetached, Reset)

	// Verify we're at the old state
	value, _ = db.Get([]byte("user:1"))
	fmt.Printf("   %suser:1%s = %s%s%s %s(original value)%s\n",
		Cyan, Reset, Green, string(value), Reset, Dim, Reset)
	value, _ = db.Get([]byte("user:3"))
	fmt.Printf("   %suser:3%s = %s%s%s %s(Charlie is back!)%s\n",
		Cyan, Reset, Green, string(value), Reset, Dim, Reset)
	fmt.Println()

	// 16. Delete a branch
	printStep(16, "Delete a branch")
	db.SwitchBranch("main") // Switch away first
	err = db.DeleteBranch("feature-x")
	if err != nil {
		log.Fatalf("Failed to delete branch: %v", err)
	}
	fmt.Printf("   Deleted branch: %s%s%s\n", Red, "feature-x", Reset)

	branches, _ = db.ListBranches()
	fmt.Printf("   Remaining branches: ")
	for i, b := range branches {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s%s%s", Green, b, Reset)
	}
	fmt.Println()
	fmt.Println()

	// Final state
	printHeader("Final State")
	branchName, isDetached, _ = db.CurrentBranch()
	fmt.Printf("Branch: %s%s%s", Green, branchName, Reset)
	if isDetached {
		fmt.Printf(" %s(detached)%s", Yellow, Reset)
	}
	fmt.Println()
	fmt.Printf("HEAD: %s%s%s\n", Yellow, shortHash(db.Head()), Reset)
	fmt.Println()
	fmt.Printf("%sKeys in working state:%s\n", Bold, Reset)
	for _, key := range []string{"user:1", "user:2", "user:3", "user:4", "user:5", "config:version"} {
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
