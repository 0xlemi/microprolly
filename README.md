# MicroProlly

A lightweight, versioned key-value store with Git-like features. Built on Prolly Trees for efficient structural sharing between versions.

## What is it?

MicroProlly is an embedded database that combines:
- **Key-Value Store**: Simple `Put`, `Get`, `Delete` operations
- **Version Control**: Commits, history, time-travel queries
- **Branching**: Create, switch, and delete branches for parallel development
- **Efficient Storage**: Only changed data is stored per version

Think of it as "Git for your data" - every change creates a new version, but unchanged data is shared between versions automatically.

## How It Works

MicroProlly uses a **Prolly Tree** (Probabilistic B-Tree) as its core data structure:

1. **Content-Based Chunking**: Data is split into chunks using a Buzhash rolling hash, creating stable boundaries
2. **Merkle Tree Properties**: Each node is identified by the SHA-256 hash of its content
3. **Structural Sharing**: Unchanged subtrees have the same hash and are automatically reused

When you change one key:
```
Version 1:          Version 2:
    Root1               Root2 (new)
    /    \              /    \
   A      B    →       A      B2 (new)
  / \    / \          / \    / \
 L1 L2  L3 L4        L1 L2  L3 L4_new (changed)
                     ↑  ↑   ↑
                   reused! reused!
```

Only the path from the changed leaf to the root is rewritten. Everything else is shared.

## Features

- **Branching**: Create parallel lines of development with named branches
- **Time Travel**: Query data as it existed at any previous commit
- **Efficient Diffs**: Compare two versions and see exactly what changed
- **Structural Sharing**: Change 1 key in 1 million? Only ~4 nodes written, not 1 million
- **Content-Addressed Storage**: Automatic deduplication via SHA-256 hashing
- **Persistence**: All data stored on disk, survives restarts
- **Atomic Writes**: Data integrity through temp file + rename pattern


## Installation

```bash
go get github.com/yourusername/microprolly
```

Or clone and use locally:

```bash
git clone https://github.com/yourusername/microprolly.git
cd microprolly
go mod tidy
```

## Complete Example

See [examples/demo/main.go](examples/demo/main.go) for a full working example that demonstrates all features including branching.

Run it with:
```bash
go run examples/demo/main.go
```



## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "microprolly/pkg/store"
)

func main() {
    // Open or create a store
    db, err := store.NewStore("./mydata")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Store some data
    db.Put([]byte("user:1"), []byte("alice"))
    db.Put([]byte("user:2"), []byte("bob"))

    // Read it back
    value, _ := db.Get([]byte("user:1"))
    fmt.Println(string(value)) // "alice"

    // Commit your changes
    commit1, _ := db.Commit("Added users")
    fmt.Printf("Commit 1: %s\n", commit1.String()[:8])
}
```

## API Reference

### Basic Operations

```go
// Put stores a key-value pair
err := db.Put(key, value)

// Get retrieves a value (returns ErrKeyNotFound if missing)
value, err := db.Get(key)

// Delete removes a key (returns ErrKeyNotFound if missing)
err := db.Delete(key)
```

### Version Control

```go
// Commit creates a snapshot with a message
commitHash, err := db.Commit("my changes")

// Log returns commit history from HEAD
commits, err := db.Log()
for _, c := range commits {
    fmt.Printf("%s: %s\n", c.Hash().String()[:8], c.Message)
}

// Head returns the current HEAD commit hash
head := db.Head()
```

### Branching

```go
// Create a new branch at current HEAD
err := db.CreateBranch("feature-x")

// Create a branch at a specific commit
err := db.CreateBranchAt("hotfix", commitHash)

// List all branches
branches, err := db.ListBranches()

// Get current branch name and detached state
name, isDetached, err := db.CurrentBranch()

// Switch to a different branch
err := db.SwitchBranch("feature-x")

// Delete a branch (cannot delete current branch)
err := db.DeleteBranch("feature-x")

// Detach HEAD to a specific commit
err := db.DetachHead(commitHash)
```

### Time Travel

```go
// GetAt retrieves a value as it existed at a specific commit
oldValue, err := db.GetAt(key, commitHash)

// Checkout restores working state to a specific commit (detaches HEAD)
err := db.Checkout(commitHash)
```

### Diff

```go
// Diff compares two commits
diff, err := db.Diff(commitA, commitB)

fmt.Println("Added:", len(diff.Added))
fmt.Println("Modified:", len(diff.Modified))
fmt.Println("Deleted:", len(diff.Deleted))
```

## Branching Model

MicroProlly follows a Git-like branching model:

- **Branches** are lightweight pointers to commits stored in `refs/heads/`
- **HEAD** tracks the current position - either attached to a branch or detached at a commit
- **Commits** automatically advance the current branch when HEAD is attached
- **Default branch** is `main`, created automatically on first store initialization

```
refs/heads/
├── main        → commit abc123
├── feature-x   → commit def456
└── hotfix      → commit 789ghi

HEAD → ref: refs/heads/main  (attached)
  or → abc123...             (detached)
```

### Branch Workflow Example

```go
// Start on main branch
db.Put([]byte("key"), []byte("value"))
db.Commit("Initial commit")

// Create and switch to feature branch
db.CreateBranch("feature")
db.SwitchBranch("feature")

// Make changes on feature branch
db.Put([]byte("feature-key"), []byte("feature-value"))
db.Commit("Add feature")

// Switch back to main - feature changes not visible
db.SwitchBranch("main")
value, err := db.Get([]byte("feature-key")) // ErrKeyNotFound

// Make independent changes on main
db.Put([]byte("main-key"), []byte("main-value"))
db.Commit("Main update")
```

## Project Structure

```
microprolly/
├── pkg/
│   ├── types/      # Core types (Hash, KVPair, Node, Commit)
│   ├── cas/        # Content-Addressed Storage
│   ├── chunker/    # Buzhash rolling hash chunking
│   ├── tree/       # Prolly Tree construction, traversal & diff
│   ├── branch/     # Branch and HEAD management
│   └── store/      # High-level Store API
├── examples/
│   └── demo/       # Working example
└── README.md
```

## On-Disk Format

```
<data_dir>/
├── objects/           # Content-addressed storage
│   ├── a1/
│   │   └── b2c3d4...  # Object files (nodes, commits)
│   └── ...
├── HEAD               # Current HEAD reference
└── refs/
    └── heads/         # Branch references
        ├── main       # Default branch
        └── ...        # Other branches
```

## Testing

MicroProlly uses property-based testing with [rapid](https://github.com/flyingmutant/rapid) to verify correctness:

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package tests
go test ./pkg/store -v
go test ./pkg/branch -v
```

## Why MicroProlly?

This is a learning project to understand how "Git for Data" systems like [Dolt](https://github.com/dolthub/dolt) work internally. It's intentionally minimal and focused on the core concepts:

- Prolly Trees for history-independent structure
- Content-addressed storage for deduplication
- Structural sharing for efficient versioning
- Git-like branching model

## Limitations

- Single-writer (no concurrent write support)
- No merging (branches can diverge but not merge)
- No garbage collection for orphaned objects
- Keys and values are byte slices (no schema)

## License

MIT
