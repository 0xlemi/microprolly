# MicroProlly: How It Works

A practical explanation of how a versioned key-value store works using Prolly Trees.

## The Big Picture

MicroProlly is a database that:
1. Stores key-value pairs (like a dictionary)
2. Tracks history (like Git)
3. Shares unchanged data between versions (efficient storage)

## Part 1: The Prolly Tree (Data Structure)

### Two Types of Nodes

**LeafNode** - Contains actual data:
```
LeafNode {
    Pairs: [
        {Key: "alice", Value: "123"},
        {Key: "bob", Value: "456"},
        {Key: "carol", Value: "789"},
    ]
}
```

**InternalNode** - Contains pointers to children:
```
InternalNode {
    Children: [
        {Key: "alice", Hash: 0xabc123...},  ← Points to a child node
        {Key: "mary",  Hash: 0xdef456...},  ← Points to another child
    ]
}
```

The `Key` is the first key in that child's subtree (used for navigation).
The `Hash` is the file path where that child is stored.

### Tree Structure

```
Root (InternalNode)
  ├─ {Key: "alice", Hash: hash_L1}
  ├─ {Key: "mary",  Hash: hash_L2}
  └─ {Key: "sue",   Hash: hash_L3}
        ↓
    LeafNode at hash_L3:
      [sue→444, tom→555, zoe→666]
```

### How Search Works

Searching for "tom":
1. Load root node
2. Compare "tom" with keys: "tom" >= "sue" → follow hash_L3
3. Load LeafNode at hash_L3
4. Binary search in pairs → found "tom" → return "555"

**Key insight:** We use alphabetical comparison on keys, not hashes!

### Building the Tree (Bottom-Up)

**Step 1: Start with sorted KV pairs**
```
alice→123, bob→456, carol→789, dave→012, eve→345, ...
```

**Step 2: Chunk using rolling hash (Buzhash)**
```
Serialize pairs and compute rolling hash as you go.
When hash % 4096 == 0 → create boundary.

Result:
  Chunk 1: [alice, bob, carol, dave]
  Chunk 2: [eve, frank, grace]
  Chunk 3: [helen, iris]
```

The `4096` is the target chunk size in **bytes** (not tree width).
This creates chunks of roughly 4KB each.

**Step 3: Create LeafNodes from chunks**
```
LeafNode1: [alice, bob, carol, dave] → hash_L1
LeafNode2: [eve, frank, grace] → hash_L2
LeafNode3: [helen, iris] → hash_L3
```

**Step 4: Create InternalNode pointing to leaves**
```
InternalNode {
    Children: [
        {Key: "alice", Hash: hash_L1},  ← First key of LeafNode1
        {Key: "eve",   Hash: hash_L2},  ← First key of LeafNode2
        {Key: "helen", Hash: hash_L3},  ← First key of LeafNode3
    ]
}
```

**Step 5: If too many nodes, chunk again!**

Keep building upward until you have one root node.

```
100,000 KV pairs
  ↓ chunk
2,500 LeafNodes
  ↓ chunk
25 InternalNodes
  ↓ chunk
1 Root
```

## Part 2: Content-Addressed Storage (CAS)

### Hash = File Path

Every node is stored as a file. The filename IS the SHA-256 hash of its content.

```
Hash: 0xabc123def456...
  ↓
File: objects/ab/c123def456...
```

The first 2 characters become a folder (for organization).

### How CAS Works

**Write:**
```go
data := node.Serialize()
hash := SHA256(data)
path := "objects/" + hash[0:2] + "/" + hash[2:]
writeFile(path, data)
return hash
```

**Read:**
```go
path := "objects/" + hash[0:2] + "/" + hash[2:]
data := readFile(path)
return deserialize(data)
```

### Automatic Deduplication

Same content → same hash → same file.

If you try to write data that already exists, it's automatically skipped!

## Part 3: Two Different Hashes

**Rolling Hash (Buzhash)** - For chunking:
- Fast, non-cryptographic
- Used to find chunk boundaries
- `hash % 4096 == 0` → boundary
- Value is discarded after chunking

**Content Hash (SHA-256)** - For node identity:
- Cryptographic, deterministic
- Used as filename in CAS
- Stored in InternalNode children
- Enables deduplication

## Part 4: Version Control (Commits)

### What is a Commit?

```go
Commit {
    RootHash:  0xabc123...,  // Points to tree root
    Message:   "Added users",
    Parent:    0xdef456...,  // Points to previous commit
    Timestamp: 1733580000,
}
```

A commit is stored in CAS just like nodes!

### HEAD Points to Current Commit

```
HEAD file contains: "333444..."
  ↓
Commit at objects/33/3444...
  ↓
Commit.RootHash = "abc123..."
  ↓
Root node at objects/ab/c123...
  ↓
Tree!
```

**HEAD → Commit → Tree Root** (not HEAD → Tree Root directly!)

### History is a Chain

```
Commit3 (HEAD)
  parent: Commit2
    ↓
Commit2
  parent: Commit1
    ↓
Commit1
  parent: null
```

Each commit points to its parent. Walk the chain to see history!

## Part 5: Structural Sharing (The Magic!)

### When You Change One Key

**Version 1:**
```
Root1
  ├─ Internal_A
  │    ├─ Leaf1 [alice, bob]
  │    └─ Leaf2 [carol, dave]
  └─ Internal_B
       ├─ Leaf3 [eve, frank]
       └─ Leaf4 [grace, helen]
```

**Version 2: Change "bob" in Leaf1**

Only the path from changed leaf to root changes:
```
Root2 (NEW - child hash changed)
  ├─ Internal_A2 (NEW - child hash changed)
  │    ├─ Leaf1_v2 (NEW - data changed)
  │    └─ Leaf2 (REUSED - same hash!)
  └─ Internal_B (REUSED - same hash!)
       ├─ Leaf3 (REUSED!)
       └─ Leaf4 (REUSED!)
```

### Why This Works

1. Change leaf → new content → new hash
2. Parent contains child hash → parent content changed → new parent hash
3. Grandparent contains parent hash → grandparent content changed → new hash
4. Continues up to root
5. **Siblings unchanged → same hash → reused automatically!**

### Files on Disk

**After Version 1:**
```
objects/
  R1, IA, IB, L1, L2, L3, L4, C1
HEAD: C1
```

**After Version 2:**
```
objects/
  R1, IA, IB, L1, L2, L3, L4, C1  ← All still here!
  R2, IA2, L1v2, C2               ← New files added
HEAD: C2
```

Nothing deleted! Old version still accessible via Commit1.

### Efficiency

Change 1 key in 1,000,000 keys:
- New nodes: ~4 (path from leaf to root)
- Reused nodes: ~24,996 (everything else)
- Old nodes: Still on disk (history preserved)

## Part 6: Time Travel

### Access Old Version

```go
// Current version
commit := loadCommit(HEAD)
root := loadNode(commit.RootHash)
value := search(root, "bob")  // Returns new value

// Old version
oldCommit := loadCommit(commit.Parent)
oldRoot := loadNode(oldCommit.RootHash)
oldValue := search(oldRoot, "bob")  // Returns old value!
```

Both trees exist on disk. Old root wasn't deleted, just "forgotten" by HEAD.

### Checkout

When you checkout an old commit:
```
HEAD: C2 → C1
```

If you don't save C2's hash somewhere, you lose access to it!
(This is why Git has branches and reflog)

## Summary

1. **Prolly Tree**: B-tree-like structure with content-based chunking
2. **CAS**: Files named by their SHA-256 hash
3. **Rolling Hash**: Determines chunk boundaries (for stability)
4. **Content Hash**: Identifies nodes (for deduplication)
5. **Commits**: Point to tree roots + parent commits
6. **HEAD**: Points to current commit
7. **Structural Sharing**: Unchanged nodes reused automatically
8. **History**: Old commits/trees stay on disk, accessible via parent chain

The magic: Same content → same hash → automatic deduplication and structural sharing!
