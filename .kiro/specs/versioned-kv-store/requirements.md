# Requirements Document

## Introduction

This document specifies the requirements for a versioned key-value store implemented in Go. The system provides Git-like version control capabilities for data, enabling time-travel queries, efficient diffs, and structural sharing through content-addressed storage. The core data structure is a Prolly Tree (Probabilistic B-Tree), which combines the efficiency of B-Trees with the history-independence of Merkle Trees.

This is a learning-focused project designed to deeply understand how "Git for Data" systems like Dolt work, while producing a useful embedded database engine.

## Glossary

- **Prolly Tree**: A Probabilistic B-Tree that uses content-based chunking (via rolling hash) to create history-independent tree structures with Merkle Tree properties.
- **Rolling Hash**: A hash function (e.g., Buzhash, Rabin-Karp) that computes a hash over a sliding window of data, used to determine chunk boundaries.
- **Chunk**: A group of key-value pairs that form a single node in the Prolly Tree, bounded by rolling hash boundaries.
- **Content-Addressed Storage (CAS)**: A storage system where data is addressed by its cryptographic hash, enabling deduplication and structural sharing.
- **Commit**: An immutable snapshot of the database state, containing a root hash, parent commit reference, message, and timestamp.
- **Root Hash**: The cryptographic hash of the root node of a Prolly Tree, uniquely identifying a complete database state.
- **Structural Sharing**: The technique of reusing unchanged nodes across versions, minimizing storage overhead.
- **Leaf Node**: A bottom-level node in the Prolly Tree containing actual key-value pairs.
- **Internal Node**: A non-leaf node containing references (hashes) to child nodes.
- **Boundary**: A position in the data stream where the rolling hash indicates a chunk should end.

## Requirements

### Requirement 1: Key-Value Operations

**User Story:** As a developer, I want to store and retrieve key-value pairs, so that I can use this as a basic data store.

#### Acceptance Criteria

1. WHEN a user calls `Put(key, value)` with a valid key and value, THEN the Versioned_KV_Store SHALL store the key-value pair in the current working state.
2. WHEN a user calls `Get(key)` for an existing key, THEN the Versioned_KV_Store SHALL return the associated value.
3. WHEN a user calls `Get(key)` for a non-existent key, THEN the Versioned_KV_Store SHALL return an error indicating the key was not found.
4. WHEN a user calls `Delete(key)` for an existing key, THEN the Versioned_KV_Store SHALL remove the key-value pair from the current working state.
5. WHEN a user calls `Delete(key)` for a non-existent key, THEN the Versioned_KV_Store SHALL return an error indicating the key was not found.

### Requirement 2: Rolling Hash Chunking

**User Story:** As a system designer, I want the data to be chunked using a rolling hash, so that the tree structure remains stable across insertions and deletions.

#### Acceptance Criteria

1. WHEN the Rolling_Hash_Chunker processes a stream of key-value pairs, THEN the Rolling_Hash_Chunker SHALL compute a rolling hash over the serialized data.
2. WHEN the rolling hash value modulo the target chunk size equals zero, THEN the Rolling_Hash_Chunker SHALL mark that position as a chunk boundary.
3. WHEN a single key-value pair is inserted into an existing dataset, THEN the Rolling_Hash_Chunker SHALL produce identical chunk boundaries for all unaffected regions.
4. WHEN the Rolling_Hash_Chunker serializes key-value pairs, THEN the Rolling_Hash_Chunker SHALL produce deterministic output for identical inputs.
5. WHEN the Rolling_Hash_Chunker serializes key-value pairs, THEN the Rolling_Hash_Chunker SHALL support a corresponding deserialization operation that recovers the original pairs (round-trip).

### Requirement 3: Prolly Tree Construction

**User Story:** As a system designer, I want the key-value pairs organized into a Prolly Tree, so that I get efficient lookups with content-addressed properties.

#### Acceptance Criteria

1. WHEN the Prolly_Tree_Builder constructs a tree from sorted key-value pairs, THEN the Prolly_Tree_Builder SHALL create leaf nodes by grouping pairs according to rolling hash boundaries.
2. WHEN the Prolly_Tree_Builder creates a node, THEN the Prolly_Tree_Builder SHALL compute a cryptographic hash (SHA-256) of the node contents as its address.
3. WHEN the Prolly_Tree_Builder has multiple leaf nodes, THEN the Prolly_Tree_Builder SHALL recursively build parent layers until a single root node remains.
4. WHEN the Prolly_Tree_Builder constructs a tree from identical sorted data, THEN the Prolly_Tree_Builder SHALL produce an identical root hash.
5. WHEN the Prolly_Tree performs a key lookup, THEN the Prolly_Tree SHALL traverse from root to leaf in O(log n) time complexity.

### Requirement 4: Content-Addressed Storage

**User Story:** As a system designer, I want all data stored by content hash, so that identical data is automatically deduplicated.

#### Acceptance Criteria

1. WHEN the CAS_Store receives data via `Write(data)`, THEN the CAS_Store SHALL compute the SHA-256 hash and store the data addressable by that hash.
2. WHEN the CAS_Store receives data that already exists (same hash), THEN the CAS_Store SHALL skip writing and return the existing hash.
3. WHEN the CAS_Store receives a `Read(hash)` request for an existing hash, THEN the CAS_Store SHALL return the original data.
4. WHEN the CAS_Store receives a `Read(hash)` request for a non-existent hash, THEN the CAS_Store SHALL return an error indicating the hash was not found.
5. WHEN the CAS_Store serializes data for storage, THEN the CAS_Store SHALL support deserialization that recovers the original data (round-trip).

### Requirement 5: Commit and Version Control

**User Story:** As a developer, I want to commit snapshots of my data with messages, so that I can track the history of changes.

#### Acceptance Criteria

1. WHEN a user calls `Commit(message)`, THEN the Versioned_KV_Store SHALL create a commit object containing the current root hash, message, timestamp, and parent commit hash.
2. WHEN a user calls `Commit(message)`, THEN the Versioned_KV_Store SHALL store the commit object in CAS and return its hash as the commit identifier.
3. WHEN a user calls `Log()`, THEN the Versioned_KV_Store SHALL return the chain of commits from the current HEAD to the initial commit.
4. WHEN a commit is created, THEN the Versioned_KV_Store SHALL serialize the commit to JSON format for storage.
5. WHEN the Versioned_KV_Store serializes a commit, THEN the Versioned_KV_Store SHALL support deserialization that recovers the original commit object (round-trip).

### Requirement 6: Time Travel Queries

**User Story:** As a developer, I want to query data as it existed at any previous commit, so that I can access historical states.

#### Acceptance Criteria

1. WHEN a user calls `Get(key, commitHash)` with a valid commit hash, THEN the Versioned_KV_Store SHALL return the value as it existed at that commit.
2. WHEN a user calls `Get(key, commitHash)` for a key that did not exist at that commit, THEN the Versioned_KV_Store SHALL return an error indicating the key was not found.
3. WHEN a user calls `Get(key, commitHash)` with an invalid commit hash, THEN the Versioned_KV_Store SHALL return an error indicating the commit was not found.
4. WHEN a user calls `Checkout(commitHash)`, THEN the Versioned_KV_Store SHALL set the working state to match that commit's data.

### Requirement 7: Efficient Diff

**User Story:** As a developer, I want to compare two commits efficiently, so that I can see what changed between versions.

#### Acceptance Criteria

1. WHEN a user calls `Diff(commitHashA, commitHashB)`, THEN the Versioned_KV_Store SHALL return the set of keys that were added, modified, or deleted between the two commits.
2. WHEN the Diff_Engine compares two trees with identical root hashes, THEN the Diff_Engine SHALL return an empty diff without traversing any nodes.
3. WHEN the Diff_Engine compares two trees, THEN the Diff_Engine SHALL skip subtrees with matching hashes (O(log n) for small changes).
4. WHEN the Diff_Engine encounters child nodes with different hashes, THEN the Diff_Engine SHALL recursively compare only those differing subtrees.

### Requirement 8: Structural Sharing

**User Story:** As a developer, I want minimal storage overhead when making small changes, so that version history is space-efficient.

#### Acceptance Criteria

1. WHEN a new version is created with a single key change, THEN the Versioned_KV_Store SHALL reuse all unchanged nodes from the previous version.
2. WHEN storing a new tree version, THEN the Versioned_KV_Store SHALL only write nodes whose hashes do not already exist in CAS.
3. WHEN multiple commits share identical subtrees, THEN the CAS_Store SHALL store only one copy of each unique node.

### Requirement 9: Persistence

**User Story:** As a developer, I want my data persisted to disk, so that it survives application restarts.

#### Acceptance Criteria

1. WHEN the Versioned_KV_Store is initialized with a directory path, THEN the Versioned_KV_Store SHALL use that directory for all CAS storage.
2. WHEN the Versioned_KV_Store is reopened from an existing directory, THEN the Versioned_KV_Store SHALL restore the previous HEAD commit and all accessible history.
3. WHEN the Versioned_KV_Store writes to disk, THEN the Versioned_KV_Store SHALL ensure data integrity through atomic write operations.

### Requirement 10: Node Serialization

**User Story:** As a system designer, I want tree nodes to be serializable, so that they can be stored and retrieved from CAS.

#### Acceptance Criteria

1. WHEN a Leaf_Node is serialized, THEN the Serializer SHALL encode all key-value pairs in a deterministic binary format.
2. WHEN an Internal_Node is serialized, THEN the Serializer SHALL encode all child references (key, hash pairs) in a deterministic binary format.
3. WHEN a serialized node is deserialized, THEN the Serializer SHALL reconstruct the original node with identical content (round-trip).
4. WHEN identical nodes are serialized, THEN the Serializer SHALL produce identical byte sequences.
