# Requirements Document

## Introduction

This document specifies the requirements for adding branching support to MicroProlly, the versioned key-value store. Branching enables parallel lines of development by allowing users to create, switch between, and manage named references to commits. This feature mirrors Git's branching model, providing a familiar workflow for version control of data.

Branching is a foundational feature that enables future capabilities like merging and collaborative workflows. This specification focuses on the core branching mechanics without merge operations.

## Glossary

- **Branch**: A named, mutable reference (pointer) to a commit hash. Branches allow parallel lines of development.
- **HEAD**: A special reference indicating the current position in the version history. HEAD can point to a branch (attached) or directly to a commit (detached).
- **Attached HEAD**: When HEAD points to a branch name, new commits automatically advance that branch.
- **Detached HEAD**: When HEAD points directly to a commit hash rather than a branch name.
- **Default Branch**: The initial branch created when the store is first initialized (typically "main").
- **Branch Reference**: A file in the refs/heads/ directory containing a commit hash.
- **Fast-Forward**: When a branch pointer is moved forward along a linear commit history.

## Requirements

### Requirement 1: Branch Creation

**User Story:** As a developer, I want to create new branches, so that I can work on parallel lines of development without affecting the main history.

#### Acceptance Criteria

1. WHEN a user calls `CreateBranch(name)` with a valid branch name, THEN the Branch_Manager SHALL create a new branch pointing to the current HEAD commit.
2. WHEN a user calls `CreateBranch(name, commitHash)` with a valid branch name and commit hash, THEN the Branch_Manager SHALL create a new branch pointing to the specified commit.
3. WHEN a user attempts to create a branch with a name that already exists, THEN the Branch_Manager SHALL return an error indicating the branch already exists.
4. WHEN a user attempts to create a branch with an invalid name (empty, contains spaces, or invalid characters), THEN the Branch_Manager SHALL return an error indicating the name is invalid.
5. WHEN a branch is created, THEN the Branch_Manager SHALL persist the branch reference to the refs/heads/ directory.

### Requirement 2: Branch Listing and Retrieval

**User Story:** As a developer, I want to list and inspect branches, so that I can understand the available lines of development.

#### Acceptance Criteria

1. WHEN a user calls `ListBranches()`, THEN the Branch_Manager SHALL return a list of all branch names.
2. WHEN a user calls `GetBranch(name)` for an existing branch, THEN the Branch_Manager SHALL return the commit hash that branch points to.
3. WHEN a user calls `GetBranch(name)` for a non-existent branch, THEN the Branch_Manager SHALL return an error indicating the branch was not found.
4. WHEN a user calls `CurrentBranch()`, THEN the Branch_Manager SHALL return the name of the current branch, or indicate detached HEAD state.

### Requirement 3: Branch Switching

**User Story:** As a developer, I want to switch between branches, so that I can work on different lines of development.

#### Acceptance Criteria

1. WHEN a user calls `SwitchBranch(name)` for an existing branch, THEN the Versioned_KV_Store SHALL update HEAD to point to that branch and load the branch's commit state into the working state.
2. WHEN a user calls `SwitchBranch(name)` for a non-existent branch, THEN the Versioned_KV_Store SHALL return an error indicating the branch was not found.
3. WHEN a user switches branches, THEN the Versioned_KV_Store SHALL update the working state to match the target branch's latest commit.
4. WHEN HEAD is in detached state and user calls `SwitchBranch(name)`, THEN the Versioned_KV_Store SHALL attach HEAD to the specified branch.

### Requirement 4: Branch Deletion

**User Story:** As a developer, I want to delete branches I no longer need, so that I can keep my branch list clean.

#### Acceptance Criteria

1. WHEN a user calls `DeleteBranch(name)` for an existing branch that is not the current branch, THEN the Branch_Manager SHALL remove the branch reference.
2. WHEN a user attempts to delete the currently checked-out branch, THEN the Branch_Manager SHALL return an error indicating the current branch cannot be deleted.
3. WHEN a user attempts to delete a non-existent branch, THEN the Branch_Manager SHALL return an error indicating the branch was not found.
4. WHEN a branch is deleted, THEN the Branch_Manager SHALL remove the branch file from refs/heads/ directory.

### Requirement 5: Commit on Branch

**User Story:** As a developer, I want commits to automatically advance the current branch, so that branch history grows naturally.

#### Acceptance Criteria

1. WHEN a user commits while HEAD points to a branch, THEN the Versioned_KV_Store SHALL update that branch to point to the new commit.
2. WHEN a user commits while HEAD is detached, THEN the Versioned_KV_Store SHALL update HEAD to point to the new commit without affecting any branch.
3. WHEN a branch is advanced by a commit, THEN the Branch_Manager SHALL atomically update the branch reference file.

### Requirement 6: Branch Persistence

**User Story:** As a developer, I want branches to persist across application restarts, so that my branch structure is preserved.

#### Acceptance Criteria

1. WHEN the Versioned_KV_Store is initialized, THEN the Branch_Manager SHALL load all existing branches from refs/heads/ directory.
2. WHEN the Versioned_KV_Store is initialized without any branches, THEN the Branch_Manager SHALL create a default "main" branch.
3. WHEN the Versioned_KV_Store is reopened, THEN the Branch_Manager SHALL restore HEAD to its previous state (branch or detached).
4. WHEN branch operations occur, THEN the Branch_Manager SHALL ensure atomic writes to prevent corruption.

### Requirement 7: HEAD State Management

**User Story:** As a developer, I want to understand and control the HEAD state, so that I know where my commits will go.

#### Acceptance Criteria

1. WHEN HEAD points to a branch, THEN the HEAD file SHALL contain the format "ref: refs/heads/{branch_name}".
2. WHEN HEAD is detached, THEN the HEAD file SHALL contain the raw commit hash.
3. WHEN a user calls `DetachHead(commitHash)`, THEN the Versioned_KV_Store SHALL set HEAD to point directly to that commit.
4. WHEN HEAD state changes, THEN the Versioned_KV_Store SHALL persist the change atomically.

