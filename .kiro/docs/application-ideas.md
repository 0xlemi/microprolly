# Application Ideas for Versioned KV Store

This document explores how to build real applications on top of the versioned key-value store, with deep dives into two primary use cases and a survey of other well-suited applications.

---

## 1. Versioned Notes CLI

A terminal-based note-taking app with full history, tagging, and search.

### Data Model

```
Keys:
  note:{uuid}           → JSON: {title, body, created, modified, tags[]}
  idx:tag:{tag}         → JSON: [uuid1, uuid2, ...]  (secondary index)
  idx:date:{YYYY-MM-DD} → JSON: [uuid1, uuid2, ...]  (secondary index)
  meta:count            → integer (total note count)
```

### Example Usage

```bash
notes add "Shopping List" "Buy milk, eggs, bread" --tags groceries,todo
notes list
notes show abc123
notes edit abc123 --body "Buy milk, eggs, bread, cheese"
notes search "milk"
notes tag abc123 urgent
notes history abc123        # show all versions of this note
notes restore abc123 @2     # restore to 2 commits ago
notes log                   # show commit history
notes diff @1 @2            # what changed between commits
```

### Implementation Strategy

**Adding a note:**
```go
func (app *NotesApp) Add(title, body string, tags []string) error {
    id := uuid.New().String()
    note := Note{
        ID: id, Title: title, Body: body,
        Tags: tags, Created: time.Now(), Modified: time.Now(),
    }
    
    // Store the note
    app.store.Put([]byte("note:"+id), note.ToJSON())
    
    // Update tag indexes
    for _, tag := range tags {
        app.updateTagIndex(tag, id, "add")
    }
    
    // Update date index
    app.updateDateIndex(time.Now(), id)
    
    // Commit automatically or let user batch
    app.store.Commit(fmt.Sprintf("Added note: %s", title))
    return nil
}
```

**Searching (full scan with filter):**
```go
func (app *NotesApp) Search(query string) []Note {
    // Option 1: Scan all notes (simple, works for <10k notes)
    allPairs, _ := app.store.GetAll()
    var results []Note
    for _, kv := range allPairs {
        if bytes.HasPrefix(kv.Key, []byte("note:")) {
            note := ParseNote(kv.Value)
            if strings.Contains(note.Body, query) || 
               strings.Contains(note.Title, query) {
                results = append(results, note)
            }
        }
    }
    return results
}
```

**History of a single note:**
```go
func (app *NotesApp) History(noteID string) []NoteVersion {
    key := []byte("note:" + noteID)
    var versions []NoteVersion
    
    // Walk commit history
    commits, _ := app.store.Log()
    for _, commit := range commits {
        value, err := app.store.GetAt(key, commit.Hash)
        if err == nil {
            versions = append(versions, NoteVersion{
                Commit:  commit,
                Content: ParseNote(value),
            })
        }
    }
    return versions
}
```

### Why This Works Well

- Notes are small, independent documents → perfect for KV
- Edit frequency is low → commit per edit is fine
- History per note is valuable → time-travel queries shine
- Tags create natural secondary indexes
- Structural sharing: editing one note doesn't duplicate others

---

## 2. Ledger Accounting CLI

A double-entry bookkeeping system inspired by [ledger-cli](https://www.ledger-cli.org/).

### The Balance Problem

You correctly identified the key challenge: **computing balances requires summing transaction history**.

Naive approach: To get "checking account balance", scan ALL transactions and sum. This is O(n) per query - unacceptable for large ledgers.

**Solution: Cached Balance Snapshots**

Store pre-computed balances at each commit. When you query a balance, you read the cached value directly.

### Data Model

```
Keys:
  tx:{timestamp}:{uuid}     → JSON: {date, description, postings[]}
  
  # Cached balances (updated on each commit)
  bal:{account}             → integer (cents) - current balance
  bal:snapshot:{commit}:{account} → integer - balance at specific commit
  
  # Account metadata
  acct:{name}               → JSON: {type, currency, parent}
  
  # Indexes
  idx:date:{YYYY-MM}        → JSON: [tx_key1, tx_key2, ...]
  idx:account:{name}        → JSON: [tx_key1, tx_key2, ...]
```

### Transaction Structure

```json
{
  "date": "2024-12-07",
  "description": "Grocery shopping",
  "postings": [
    {"account": "expenses:food", "amount": 5000},
    {"account": "assets:checking", "amount": -5000}
  ]
}
```

Note: Amounts in cents (integer) to avoid floating point issues. Postings must sum to zero (double-entry).

### Implementation Strategy

**Adding a transaction:**
```go
func (app *LedgerApp) AddTransaction(tx Transaction) error {
    // Validate double-entry (sum of postings == 0)
    var sum int64
    for _, p := range tx.Postings {
        sum += p.Amount
    }
    if sum != 0 {
        return errors.New("transaction does not balance")
    }
    
    // Store transaction
    key := fmt.Sprintf("tx:%s:%s", tx.Date.Format(time.RFC3339), uuid.New())
    app.store.Put([]byte(key), tx.ToJSON())
    
    // Update cached balances
    for _, p := range tx.Postings {
        app.updateBalance(p.Account, p.Amount)
    }
    
    // Update indexes
    app.updateDateIndex(tx.Date, key)
    for _, p := range tx.Postings {
        app.updateAccountIndex(p.Account, key)
    }
    
    return nil
}

func (app *LedgerApp) updateBalance(account string, delta int64) {
    key := []byte("bal:" + account)
    current, err := app.store.Get(key)
    
    var balance int64
    if err == nil {
        balance = bytesToInt64(current)
    }
    balance += delta
    
    app.store.Put(key, int64ToBytes(balance))
}
```

**Getting current balance (O(1)):**
```go
func (app *LedgerApp) Balance(account string) (int64, error) {
    key := []byte("bal:" + account)
    data, err := app.store.Get(key)
    if err != nil {
        return 0, err
    }
    return bytesToInt64(data), nil
}
```

**Getting historical balance:**
```go
func (app *LedgerApp) BalanceAt(account string, commitHash Hash) (int64, error) {
    // Option 1: Read cached snapshot if we stored it
    snapshotKey := fmt.Sprintf("bal:snapshot:%x:%s", commitHash, account)
    data, err := app.store.GetAt([]byte(snapshotKey), commitHash)
    if err == nil {
        return bytesToInt64(data), nil
    }
    
    // Option 2: Recompute from transactions (fallback)
    return app.recomputeBalanceAt(account, commitHash)
}
```

**Commit with balance snapshots:**
```go
func (app *LedgerApp) Commit(message string) (Hash, error) {
    // Before committing, snapshot all balances
    // This makes historical queries O(1)
    accounts := app.getAllAccounts()
    commitHash := app.store.PendingCommitHash() // preview
    
    for _, acct := range accounts {
        bal, _ := app.Balance(acct)
        snapshotKey := fmt.Sprintf("bal:snapshot:%x:%s", commitHash, acct)
        app.store.Put([]byte(snapshotKey), int64ToBytes(bal))
    }
    
    return app.store.Commit(message)
}
```

### Example Usage

```bash
ledger add "2024-12-07" "Paycheck" \
    income:salary -500000 \
    assets:checking 500000

ledger add "2024-12-07" "Groceries" \
    expenses:food 5000 \
    assets:checking -5000

ledger balance                    # show all account balances
ledger balance assets:checking    # show specific account
ledger balance --at @3            # balance 3 commits ago
ledger register assets:checking   # show all transactions for account
ledger report monthly             # monthly summary
ledger diff @1 @2                 # what transactions changed
```

### Why This Works Well

- Transactions are immutable facts → append-only is natural
- Audit trail is critical → version history is the audit log
- "What was the balance on Dec 1?" → time-travel query
- Cached balances solve the O(n) problem
- Structural sharing: adding transactions doesn't duplicate old ones

### Trade-off: Storage vs Compute

The balance snapshot approach trades storage for query speed:
- Without snapshots: O(n) to compute any historical balance
- With snapshots: O(1) query, but ~O(accounts) extra storage per commit

For a personal ledger (hundreds of accounts, thousands of transactions), this is negligible. For enterprise scale, you might only snapshot monthly.

---

## 3. Other Well-Suited Applications

### Configuration/Dotfiles Manager

```
Keys:
  file:{path}          → file contents
  meta:files           → JSON: [path1, path2, ...]
  meta:machine:{name}  → JSON: {os, paths[]}
```

Why it fits:
- Config files are usually small text
- "What changed in my .zshrc?" → diff
- Rollback broken config → checkout
- Sync between machines (future)
- Machine-to-machine: deploy configs to servers, track what changed

---

### Password/Secrets Manager

```
Keys:
  secret:{uuid}        → encrypted JSON: {name, value, metadata}
  idx:service:{name}   → [uuid1, uuid2]
  meta:master_hash     → hash for verification
```

Why it fits:
- History is critical ("what was the old API key before rotation?")
- Audit trail: who changed what, when
- Sync between machines with conflict detection
- Machine-to-machine: services fetching credentials

---

### Feature Flags / Remote Config Service

```
Keys:
  flag:{name}              → JSON: {enabled, rollout_percent, conditions[]}
  config:{app}:{key}       → JSON: {value, type, description}
  idx:app:{name}           → [key1, key2, ...]
  audit:{timestamp}:{flag} → JSON: {old_value, new_value, changed_by}
```

Why it fits:
- Flags change frequently, history matters ("when did we enable dark mode?")
- Rollback bad config instantly → checkout
- Diff between deployments: "what flags changed in this release?"
- Machine-to-machine: apps poll for config, get specific version
- A/B testing: track which config was active at any point

---

### Infrastructure State Store (like Terraform state)

```
Keys:
  resource:{provider}:{type}:{name} → JSON: {id, attributes, dependencies[]}
  output:{name}                     → JSON: {value, sensitive}
  meta:serial                       → integer (state version)
  lock:{workspace}                  → JSON: {holder, timestamp}
```

Why it fits:
- Infrastructure state is THE use case for versioned storage
- "What did our infra look like before the outage?" → time-travel
- Diff between states: "what resources changed?"
- Structural sharing: changing one resource doesn't duplicate entire state
- Machine-to-machine: CI/CD pipelines read/write state

---

### Event Sourcing Store

```
Keys:
  event:{aggregate}:{sequence}  → JSON: {type, payload, timestamp, metadata}
  snapshot:{aggregate}          → JSON: {state, at_sequence}
  idx:type:{event_type}         → [event_key1, event_key2, ...]
```

Why it fits:
- Events are immutable facts → append-only is natural
- Snapshots = cached aggregations (like ledger balances)
- Replay from any point: checkout old commit, replay events
- Machine-to-machine: services publish/consume events
- "What was the order state at 3pm?" → time-travel query

---

### API Response Cache with Versioning

```
Keys:
  cache:{endpoint_hash}     → JSON: {response, headers, cached_at}
  meta:ttl:{endpoint_hash}  → integer (seconds)
  idx:domain:{domain}       → [endpoint_hash1, ...]
```

Why it fits:
- Cache invalidation is hard; versioning helps debug
- "What response were we serving yesterday?" → time-travel
- Diff: "how did the API response change?"
- Machine-to-machine: services share cached responses
- Rollback: serve old cached response if new one is broken

---

### Schema Registry (like Confluent Schema Registry)

```
Keys:
  schema:{subject}:{version}  → JSON/Avro/Protobuf schema definition
  latest:{subject}            → integer (latest version number)
  compat:{subject}            → string (compatibility mode)
  idx:type:{schema_type}      → [subject1, subject2, ...]
```

Why it fits:
- Schemas evolve; history is the whole point
- Compatibility checking: compare new schema against history
- "What schema was producer X using last week?" → time-travel
- Machine-to-machine: producers/consumers fetch schemas
- Structural sharing: similar schemas share common parts

---

### Distributed Lock/Lease Manager

```
Keys:
  lock:{resource}       → JSON: {holder, acquired_at, ttl, metadata}
  lease:{resource}      → JSON: {holder, renewed_at, ttl}
  history:{resource}    → JSON: [{holder, from, to}, ...]
```

Why it fits:
- Lock history is valuable for debugging deadlocks
- "Who held this lock during the incident?" → time-travel
- Machine-to-machine: services acquire/release locks
- Audit trail built-in

---

### ML Model/Artifact Registry

```
Keys:
  model:{name}:{version}      → binary (model weights) or reference to blob store
  metadata:{name}:{version}   → JSON: {metrics, params, created_by, dataset}
  latest:{name}               → version string
  idx:tag:{tag}               → [model_key1, model_key2, ...]
```

Why it fits:
- Model versioning is critical for reproducibility
- "What model was serving predictions on Dec 1?" → time-travel
- Diff: compare metadata between versions
- Rollback: revert to previous model version
- Machine-to-machine: inference services fetch models

---

## Summary: What Makes a Good Fit?

| Characteristic     | Good Fit                 | Poor Fit                              |
| ------------------ | ------------------------ | ------------------------------------- |
| Document size      | Small (<1MB)             | Large binary blobs                    |
| Access pattern     | By key/ID                | Complex queries (joins, aggregations) |
| Change frequency   | Occasional edits         | High-frequency writes                 |
| History value      | Important                | Irrelevant                            |
| Data relationships | Loose (tags, indexes)    | Tight (foreign keys)                  |
| Consistency needs  | Eventually consistent OK | ACID transactions required            |

The versioned KV store excels at **document-oriented, history-aware applications** where data is naturally addressable by a key and changes are incremental.
