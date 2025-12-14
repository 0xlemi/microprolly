# MicroProlly Roadmap

A versioned key-value store with Git-like branching, event sourcing principles, and automatic projections.

## Vision

MicroProlly combines:
- **Event Sourcing**: Commits are immutable events, the source of truth
- **Projections**: Materialized views derived from data, queryable
- **Branching**: Parallel realities, each with their own projections
- **Authorization**: Security baked into the data layer (optional)

```
Events/Commits (immutable)
       ↓
   Schema (optional, defines structure)
       ↓
Projections (versioned, queryable)
       ↓
  Branches (parallel events + projection states)
```

---

## Phase 1: Core Git Features

Complete the Git-like foundation before adding projections.

### 1.1 Merge
Three-way merge using common ancestor, with conflict detection.

**Why:** Branches are useless without merge. This completes the branching story.

**API:**
```go
result, err := db.Merge("feature-x", "main")
// result.Conflicts - keys modified on both branches
// result.Merged - auto-merged keys
```

**Approach:**
- Find common ancestor (LCA of two commits)
- Three-way diff: ancestor↔A, ancestor↔B
- Auto-merge non-conflicting changes
- Return conflicts for manual resolution

---

### 1.2 Range Queries
Efficient prefix and range scans.

**Why:** Required for projections. Prolly trees are sorted, so this is natural.

**API:**
```go
pairs, _ := db.GetRange([]byte("user:"), []byte("user:~"))
pairs, _ := db.GetPrefix([]byte("user:"))
```

---

### 1.3 Tags
Immutable named references to commits.

**Why:** Quick win, useful for marking versions/releases.

**API:**
```go
db.CreateTag("v1.0", commitHash)
db.ListTags()
db.GetTag("v1.0")
```

---

### 1.4 Garbage Collection
Mark-and-sweep to reclaim orphaned objects.

**Why:** Necessary for production use. Without GC, disk grows forever.

**API:**
```go
stats, _ := db.GC()
// stats.ObjectsRemoved, stats.BytesReclaimed
```

---

### 1.5 Stash
Save uncommitted working state temporarily.

**Why:** Useful when you need to switch branches but have uncommitted work.

**API:**
```go
stashId, _ := db.Stash("WIP: fixing bug")
db.SwitchBranch("hotfix")
// ... do other work ...
db.SwitchBranch("feature")
db.StashPop(stashId)

// List stashes
stashes, _ := db.ListStashes()
```

---

### 1.6 Compression
Compress object storage (zstd or snappy).

**Why:** Easy win for disk space, especially for text-heavy data.

**Approach:**
- Compress before writing to CAS
- Decompress on read
- Store compression flag in object header
- Optional: configurable compression level

**API:**
```go
db, _ := store.NewStore("./data", store.WithCompression("zstd"))
```

---

### 1.7 Hooks
Pre-commit and post-commit callbacks.

**Why:** Enforce invariants, trigger notifications, validate data before commit.

**API:**
```go
db.OnPreCommit(func(changes []Change) error {
    // Validate changes, return error to abort commit
    for _, c := range changes {
        if !isValid(c.Value) {
            return errors.New("invalid data")
        }
    }
    return nil
})

db.OnPostCommit(func(commit Commit) {
    // Notify, log, trigger side effects
    log.Printf("Committed: %s", commit.Hash)
})
```

---

## Phase 2: Schema & Projections

### 2.1 Schema Definition
Define types with fields and indexes.

**Why:** Database needs to understand data structure to build projections.

**Design Decisions:**
- Values are JSON (human-readable, widely supported)
- Schema is OPTIONAL (untyped keys still work as pure KV)
- Schema validated on write
- Schema is global (not per-branch) to avoid complexity

**API:**
```go
db.DefineType("user", Schema{
    Fields: []Field{
        {Name: "id", Type: "string", Required: true},
        {Name: "name", Type: "string"},
        {Name: "email", Type: "string"},
        {Name: "role", Type: "string"},
        {Name: "created_at", Type: "timestamp"},
    },
})

// Keys matching pattern are validated
// Pattern: {type}:{id}
db.Put([]byte("user:123"), []byte(`{"id":"123","name":"Alice","role":"admin"}`))

// Untyped keys still work (no schema validation)
db.Put([]byte("config:setting"), []byte("value"))
```

**Supported Types (initial):**
- `string`
- `number`
- `boolean`
- `timestamp`
- `array` (of primitives)

---

### 2.2 Basic Projections (Indexes)
Auto-maintained indexes for fast lookups.

**Why:** Event stores are useless for querying. Projections make data queryable.

**Design Decisions:**
- Projections are versioned (stored per-commit)
- Speed over space - projections are pre-computed
- Projections update on commit, not on put

**API:**
```go
// Create index projection
db.CreateProjection("users_by_role", Projection{
    Source:  "user:*",
    IndexBy: "role",
})

// Query projection
results, _ := db.QueryProjection("users_by_role", "admin")
// Returns: [user:123, user:456, ...]
```

**Behavior on branch switch:**
```go
// main: user:1 has role "admin"
// feature: user:1 has role "viewer"

db.SwitchBranch("main")
db.QueryProjection("users_by_role", "admin")  // [user:1]

db.SwitchBranch("feature")
db.QueryProjection("users_by_role", "admin")  // []
```

---

### 2.3 Compound Projections
Index by multiple fields.

**API:**
```go
db.CreateProjection("users_by_role_and_status", Projection{
    Source:  "user:*",
    IndexBy: []string{"role", "status"},
})

db.QueryProjection("users_by_role_and_status", "admin", "active")
```

---

### 2.4 Filter Projections
Projections with conditions.

**API:**
```go
db.CreateProjection("active_admins", Projection{
    Source:  "user:*",
    Filter:  "role = 'admin' AND status = 'active'",
    IndexBy: "name",
})
```

---

### 2.5 Schema Changes & Projection Rebuilding

Schema changes affect all projections across all branches and commits. This is handled with **lazy rebuild + background queue**.

**The Problem:**
```
You have:
- 5 branches
- 100 commits total
- Each commit has projection snapshots

Schema change (e.g., add new index field) → potentially 100 projection snapshots need rebuild
```

**Solution: Lazy Rebuild + Background Queue**

1. **Schema change is instant** - Don't block on rebuild
2. **Mark affected projections as "stale"** - Track which commit/projection combos need rebuild
3. **Lazy rebuild on access** - If you query a stale projection, rebuild it first
4. **Background queue** - Worker rebuilds stale projections async
5. **Status visibility** - API shows rebuild progress

**How it works:**

```go
// 1. Schema change marks projections stale
db.CreateProjection("users_by_status", Projection{
    Source:  "user:*",
    IndexBy: "status",  // New index
})
// Returns immediately
// Internally: marks all commits as needing rebuild for this projection

// 2. Query on current branch - lazy rebuild
results, _ := db.QueryProjection("users_by_status", "active")
// If stale: rebuilds projection for current commit, then returns
// If fresh: returns immediately

// 3. Background worker rebuilds other commits/branches
// Prioritizes: current branch > other branches > old commits

// 4. Check rebuild status
status, _ := db.ProjectionStatus("users_by_status")
// status.TotalCommits: 100
// status.Rebuilt: 45
// status.Pending: 55
// status.IsRebuilding: true
```

**Stale Projection Behavior:**

| Scenario                               | Behavior                                       |
| -------------------------------------- | ---------------------------------------------- |
| Query current commit, projection stale | Rebuild sync, then return (may be slow)        |
| Query current commit, projection fresh | Return immediately                             |
| Query old commit, projection stale     | Rebuild sync OR return error with "rebuilding" |
| Switch branch, projection stale        | Rebuild current commit sync                    |

**Rebuild Priority Queue:**

```
Priority 1: Current branch HEAD (blocking - user is waiting)
Priority 2: Current branch history (background)
Priority 3: Other branches HEAD (background)
Priority 4: Other branches history (background, low priority)
```

**API for rebuild control:**

```go
// Force immediate rebuild of everything (blocking)
db.RebuildProjections("users_by_status")

// Cancel pending rebuilds (e.g., if dropping projection anyway)
db.CancelRebuild("users_by_status")

// Pause/resume background rebuilding
db.PauseBackgroundRebuilds()
db.ResumeBackgroundRebuilds()
```

**Storage of stale markers:**

```
<data_dir>/
├── projections/
│   ├── users_by_role/
│   │   ├── <commit_hash_1>  # projection data
│   │   ├── <commit_hash_2>  # projection data
│   │   └── ...
│   └── _stale/
│       └── users_by_status  # list of commits needing rebuild
```

---

## Phase 3: Observability & Recovery

### 3.1 Metrics & Stats
Understand what's happening inside the database.

**Why:** "Why is this slow?" needs answers. Production systems need observability.

**API:**
```go
stats, _ := db.Stats()
// stats.TotalKeys
// stats.TotalCommits
// stats.TotalBranches
// stats.ObjectCount
// stats.DiskUsage
// stats.ProjectionStats (per projection)

// Per-operation timing
db.EnableMetrics()
metrics := db.Metrics()
// metrics.AvgReadLatency
// metrics.AvgWriteLatency
// metrics.AvgCommitLatency
```

---

### 3.2 Repair / FSCK
Detect and fix corruption.

**Why:** What if rebuild crashes halfway? What if disk corrupts? Need recovery tools.

**API:**
```go
// Check integrity
issues, _ := db.Check()
// issues: []Issue{Type: "orphaned_object", Hash: "abc123"}
// issues: []Issue{Type: "missing_object", Hash: "def456"}
// issues: []Issue{Type: "corrupt_projection", Name: "users_by_role"}

// Attempt repair
repaired, _ := db.Repair()
// repaired.OrphanedObjectsRemoved
// repaired.ProjectionsRebuilt
// repaired.Errors (things that couldn't be fixed)
```

---

### 3.3 Debug / Inspect Tools
Understand the data structure.

**API:**
```go
// Inspect a commit
info, _ := db.InspectCommit(hash)
// info.Parent, info.RootHash, info.Message, info.Timestamp

// Inspect tree structure
tree, _ := db.InspectTree(rootHash)
// tree.Depth, tree.NodeCount, tree.LeafCount

// Dump object (for debugging)
obj, _ := db.DumpObject(hash)
// Raw bytes + parsed structure
```

---

## Phase 4: Authorization

### 4.1 Token-Based Auth
Security baked into the database layer.

**Why:** With branches, commits, merges - many operations need auth. Better in DB than middleware.

**Design:**
- Token contains: user ID, roles, permissions
- Key ownership by prefix
- Branch permissions
- Operation permissions (read, write, delete, merge)

**API:**
```go
// Open with auth context
db, _ := store.NewStore("./data", store.WithAuth(token))

// Token: { user: "alice", roles: ["editor"] }

// Key ownership
db.Put([]byte("alice:notes:1"), data)     // OK - alice owns alice:*
db.Put([]byte("bob:notes:1"), data)       // Error - unauthorized

// Role-based access
db.Put([]byte("shared:docs:1"), data)     // OK if "editor" role has access
```

---

### 4.2 Permission Rules
Define access rules in the database.

**API:**
```go
db.DefinePermission(Permission{
    Pattern: "user:{userId}:*",
    Owner:   "{userId}",           // Owner extracted from key
    Roles:   map[string][]string{
        "admin": {"read", "write", "delete"},
    },
})

db.DefinePermission(Permission{
    Pattern: "shared:*",
    Roles:   map[string][]string{
        "editor": {"read", "write"},
        "viewer": {"read"},
    },
})
```

---

### 4.3 Branch Permissions
Control who can create, merge, delete branches.

**API:**
```go
db.DefineBranchPermission(BranchPermission{
    Pattern: "main",
    Merge:   []string{"admin"},        // Only admins can merge to main
    Delete:  []string{},               // No one can delete main
})

db.DefineBranchPermission(BranchPermission{
    Pattern: "{user}/*",               // e.g., alice/feature-x
    Owner:   "{user}",                 // User owns their branches
})
```

---

## Phase 5: Advanced Projections (Future / Maybe)

Features for later, once basics are solid. May or may not be implemented.

### 5.1 Aggregation Projections
Compute aggregates (count, sum, avg).

```go
db.CreateProjection("orders_stats_by_user", Projection{
    Source:    "order:*",
    GroupBy:   "user_id",
    Aggregate: map[string]string{
        "total_orders": "COUNT(*)",
        "total_spent":  "SUM(amount)",
        "avg_order":    "AVG(amount)",
    },
})
```

---

### 5.2 Join Projections
Denormalized views joining multiple types.

```go
db.CreateProjection("orders_with_user", Projection{
    Source: "order:*",
    Join: Join{
        Type:       "user:*",
        LocalKey:   "user_id",
        ForeignKey: "id",
        Fields:     []string{"name", "email"},
    },
})
```

---

### 5.3 Computed Fields
Derive fields from existing data.

```go
db.CreateProjection("users_enriched", Projection{
    Source: "user:*",
    Computed: map[string]string{
        "full_name":    "first_name || ' ' || last_name",
        "is_premium":   "plan = 'premium' OR plan = 'enterprise'",
    },
})
```

---

### 5.4 Real-time Subscriptions
Watch for changes to projections.

```go
ch := db.WatchProjection("active_admins")
for change := range ch {
    // change.Added, change.Removed, change.Modified
}
```

---

## Phase 6: Query Language (Future / Maybe)

A simple query language for projections. Big undertaking - may never happen.

```sql
-- Basic query
SELECT * FROM user WHERE role = 'admin'

-- Time travel
SELECT * FROM user AS OF 'abc123' WHERE role = 'admin'

-- Branch query
SELECT * FROM user ON BRANCH 'feature-x' WHERE role = 'admin'

-- Diff
DIFF 'main' 'feature-x' ON user WHERE role = 'admin'
```

---

## Phase 7: Advanced Git Features (Future / Maybe)

### 7.1 Rebase
Replay commits from one branch onto another for linear history.

```go
err := db.Rebase("feature", "main")
```

---

### 7.2 Cherry-pick
Apply a specific commit to the current branch.

```go
err := db.CherryPick(commitHash)
```

---

### 7.3 Blame
For each key, show which commit last modified it.

```go
info, _ := db.Blame([]byte("user:123"))
// info.CommitHash, info.Author, info.Timestamp, info.Message
```

---

### 7.4 Remote Sync
Push/pull between stores for collaboration and backup.

```go
remote := store.NewRemote("https://example.com/db")
db.Push(remote, "main")
db.Pull(remote, "main")
```

---

### 7.5 CRDT-based Merge
Conflict-free merging for specific data types (counters, sets).

```go
db.RegisterCRDT("counter:*", crdt.GCounter)
// Merges automatically resolve using CRDT semantics
```

---

### 7.6 Transactions
Multi-key atomic operations with rollback.

```go
tx := db.Begin()
tx.Put([]byte("account:A"), []byte("900"))
tx.Put([]byte("account:B"), []byte("1100"))
err := tx.Commit()  // Atomic
// or tx.Rollback()
```

---

## Implementation Order

| Priority | Feature              | Depends On            | Phase |
| -------- | -------------------- | --------------------- | ----- |
| 1        | Range Queries        | -                     | 1     |
| 2        | Merge                | -                     | 1     |
| 3        | Tags                 | -                     | 1     |
| 4        | Garbage Collection   | -                     | 1     |
| 5        | Stash                | -                     | 1     |
| 6        | Compression          | -                     | 1     |
| 7        | Hooks                | -                     | 1     |
| 8        | Schema Definition    | -                     | 2     |
| 9        | Basic Projections    | Schema, Range Queries | 2     |
| 10       | Compound Projections | Basic Projections     | 2     |
| 11       | Filter Projections   | Basic Projections     | 2     |
| 12       | Metrics & Stats      | -                     | 3     |
| 13       | Repair / FSCK        | -                     | 3     |
| 14       | Token-Based Auth     | -                     | 4     |
| 15       | Permission Rules     | Token-Based Auth      | 4     |
| 16       | Branch Permissions   | Permission Rules      | 4     |
| 17+      | Advanced Projections | Basic Projections     | 5     |
| 18+      | Query Language       | Projections           | 6     |
| 19+      | Advanced Git         | Merge                 | 7     |

---

## What Makes This Unique

No existing database combines all of:

| Feature               | EventStoreDB | Dolt | Firebase | MicroProlly |
| --------------------- | ------------ | ---- | -------- | ----------- |
| Immutable events      | ✓            | ✓    | -        | ✓           |
| Branching             | -            | ✓    | -        | ✓           |
| Merge                 | -            | ✓    | -        | ✓           |
| Auto projections      | ✓            | -    | -        | ✓           |
| Versioned projections | -            | -    | -        | ✓           |
| Built-in auth         | -            | -    | ✓        | ✓           |
| Branch-aware auth     | -            | -    | -        | ✓           |

The killer feature: **Projections that change when you switch branches.**

```go
db.SwitchBranch("experiment")
// All projections now reflect experiment branch state
// No manual rebuilding, no stale data
```

This enables:
- A/B testing on data
- "What-if" analysis
- Safe experimentation
- Audit trails with queryable views

---

## Scope Warning

This roadmap describes an ambitious multi-year project. The goal is:

**Phase 1-2**: A really good versioned KV store with projections (6-12 months)
**Phase 3-4**: Production-ready with observability and auth (6-12 months)
**Phase 5-6**: Maybe never - depends on real-world needs

Build Phase 1-2 well. Ship it. See what users actually need before building more.
