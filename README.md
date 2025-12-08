# MicroProlly

A lightweight, versioned key-value store with Git-like features. Built on Prolly Trees for efficient structural sharing between versions.

## What is it?

MicroProlly is an embedded database that combines:
- **Key-Value Store**: Simple `Put`, `Get`, `Delete` operations
- **Version Control**: Commits, history, time-travel queries
- **Efficient Storage**: Only changed data is stored per version

Think of it as "Git for your data" - every change creates a new version, but unchanged data is shared between versions automatically.

## Features

- **Time Travel**: Query data as it existed at any previous commit
- **Efficient Diffs**: Compare two versions and see exactly what changed
- **Structural Sharing**: Change 1 key in 1 million? Only ~4 nodes written, not 1 million
- **Content-Addressed Storage**: Automatic deduplication
- **Persistence**: All data stored on disk, survives restarts

## Usage

```go
package main

import (
    "fmt"
    "microprolly/pkg/store"
)

func main() {
    // Open or create a store
    db, _ := store.Open("./mydata")
    defer db.Close()

    // Basic operations
    db.Put([]byte("user:1"), []byte("alice"))
    db.Put([]byte("user:2"), []byte("bob"))

    value, _ := db.Get([]byte("user:1"))
    fmt.Println(string(value)) // "alice"

    // Commit your changes
    commit1, _ := db.Commit("Added users")

    // Make more changes
    db.Put([]byte("user:1"), []byte("alice_updated"))
    db.Delete([]byte("user:2"))
    commit2, _ := db.Commit("Updated alice, removed bob")

    // Time travel: query old version
    oldValue, _ := db.GetAt([]byte("user:1"), commit1)
    fmt.Println(string(oldValue)) // "alice" (original value!)

    // See what changed between versions
    diff, _ := db.Diff(commit1, commit2)
    fmt.Println(diff.Modified) // ["user:1"]
    fmt.Println(diff.Deleted)  // ["user:2"]

    // View history
    commits, _ := db.Log()
    for _, c := range commits {
        fmt.Printf("%s: %s\n", c.Hash, c.Message)
    }

    // Checkout old version
    db.Checkout(commit1)
    value, _ = db.Get([]byte("user:1"))
    fmt.Println(string(value)) // "alice" (back to original!)
}
```

## How It Works

MicroProlly uses a **Prolly Tree** (Probabilistic B-Tree) as its core data structure:

1. **Content-Based Chunking**: Data is split into chunks using a rolling hash, creating stable boundaries
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

## Project Structure

```
microprolly/
├── pkg/
│   ├── types/      # Core types (Hash, KVPair, Node, Commit)
│   ├── cas/        # Content-Addressed Storage
│   ├── chunker/    # Rolling hash chunking
│   ├── tree/       # Prolly Tree construction & traversal
│   └── store/      # High-level Store API
└── README.md
```

## Why MicroProlly?

This is a learning project to understand how "Git for Data" systems like [Dolt](https://github.com/dolthub/dolt) work internally. It's intentionally minimal and focused on the core concepts:

- Prolly Trees for history-independent structure
- Content-addressed storage for deduplication
- Structural sharing for efficient versioning

## Status

🚧 **Work in Progress** - Core functionality is being implemented.

## License

MIT
