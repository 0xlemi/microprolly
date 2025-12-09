# Node Serialization Format

This document describes the binary serialization format used for Prolly Tree nodes in MicroProlly. The format is deterministic, meaning identical nodes always produce identical byte sequences, which is essential for content-addressed storage and structural sharing.

## Overview

Nodes are serialized to binary format before being stored in the Content-Addressed Storage (CAS). The serialization uses:
- **Big-endian encoding** for all multi-byte integers (for cross-platform determinism)
- **Length-prefixed data** for variable-length fields (keys, values)
- **Type byte prefix** to distinguish between leaf and internal nodes

## Node Types

- `0x01` - Leaf Node (contains key-value pairs)
- `0x02` - Internal Node (contains child references)

---

## Leaf Node Format

A leaf node contains actual key-value pairs that store the data.

### Structure

```
┌──────────────────────────────────────────────────────────────┐
│                      LEAF NODE LAYOUT                         │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  [Node Type] [Pair Count] [Pair 1] [Pair 2] ... [Pair N]    │
│   1 byte      4 bytes      variable  variable     variable   │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

### Header (5 bytes)

| Offset | Size | Field      | Type   | Description                  |
| ------ | ---- | ---------- | ------ | ---------------------------- |
| 0      | 1    | Node Type  | uint8  | Always `0x01` for leaf nodes |
| 1      | 4    | Pair Count | uint32 | Number of key-value pairs    |

### Key-Value Pair Format (repeated for each pair)

| Field        | Size    | Type   | Description                  |
| ------------ | ------- | ------ | ---------------------------- |
| Key Length   | 4 bytes | uint32 | Length of the key in bytes   |
| Key Data     | N bytes | []byte | The actual key bytes         |
| Value Length | 4 bytes | uint32 | Length of the value in bytes |
| Value Data   | M bytes | []byte | The actual value bytes       |

### Visual Example

Let's serialize a leaf node with 2 pairs:
- Pair 1: `key="user"`, `value="alice"`
- Pair 2: `key="age"`, `value="25"`

```
Byte Position:  0      1    2    3    4      5    6    7    8      9   10   11   12
               ┌───┬────────────────────┬──────────────────────┬─────────────────────
               │01 │ 00   00   00   02 │ 00   00   00   04    │ 75  73  65  72  
               └───┴────────────────────┴──────────────────────┴─────────────────────
                 │           │                    │                    │
            Node Type    Pair Count          Key Length            Key Data
            = 0x01       = 2                 = 4                  = "user"
            (leaf)       (big-endian)        (big-endian)         (UTF-8)


               13   14   15   16     17   18   19   20   21     22   23   24   25
              ┬──────────────────┬────────────────────────┬──────────────────────┬───
              │ 00   00   00   05│ 61  6C  69  63  65    │ 00   00   00   03    │ 61
              ┴──────────────────┴────────────────────────┴──────────────────────┴───
                      │                    │                       │                │
                Value Length          Value Data              Key Length         Key Data
                = 5                   = "alice"               = 3                = "age"
                (big-endian)          (UTF-8)                 (big-endian)


               26   27     28   29   30   31     32   33
              ┬─────────┬──────────────────────┬────────────────
              │ 67  65  │ 00   00   00   02    │ 32  35
              ┴─────────┴──────────────────────┴────────────────
                   │              │                   │
              Key Data      Value Length         Value Data
              = "ge"        = 2                  = "25"
                            (big-endian)         (UTF-8)
```

### Breakdown by Section

```
┌──────────────────────────────────────────────────────────────┐
│ SECTION 1: HEADER (5 bytes)                                  │
├──────────────────────────────────────────────────────────────┤
│ Byte 0:       0x01 (node type = leaf)                        │
│ Bytes 1-4:    0x00000002 (pair count = 2)                    │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ SECTION 2: FIRST KEY-VALUE PAIR (17 bytes)                   │
├──────────────────────────────────────────────────────────────┤
│ Bytes 5-8:    0x00000004 (key length = 4)                    │
│ Bytes 9-12:   0x75736572 ("user" in hex)                     │
│ Bytes 13-16:  0x00000005 (value length = 5)                  │
│ Bytes 17-21:  0x616C696365 ("alice" in hex)                  │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ SECTION 3: SECOND KEY-VALUE PAIR (13 bytes)                  │
├──────────────────────────────────────────────────────────────┤
│ Bytes 22-25:  0x00000003 (key length = 3)                    │
│ Bytes 26-28:  0x616765 ("age" in hex)                        │
│ Bytes 29-32:  0x00000002 (value length = 2)                  │
│ Bytes 33-34:  0x3235 ("25" in hex)                           │
└──────────────────────────────────────────────────────────────┘

TOTAL SIZE: 5 + 17 + 13 = 35 bytes
```

### Size Calculation

For a leaf node with N pairs:
```
Total Size = 5 + Σ(4 + key_length[i] + 4 + value_length[i]) for i=1 to N
           = 5 + Σ(8 + key_length[i] + value_length[i])
```

---

## Internal Node Format

An internal node contains references (hashes) to child nodes, forming the tree structure.

### Structure

```
┌──────────────────────────────────────────────────────────────┐
│                    INTERNAL NODE LAYOUT                       │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  [Node Type] [Child Count] [Child 1] [Child 2] ... [Child N]│
│   1 byte      4 bytes       variable   variable     variable │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

### Header (5 bytes)

| Offset | Size | Field       | Type   | Description                      |
| ------ | ---- | ----------- | ------ | -------------------------------- |
| 0      | 1    | Node Type   | uint8  | Always `0x02` for internal nodes |
| 1      | 4    | Child Count | uint32 | Number of child references       |

### Child Reference Format (repeated for each child)

| Field      | Size     | Type     | Description                        |
| ---------- | -------- | -------- | ---------------------------------- |
| Key Length | 4 bytes  | uint32   | Length of the first key in child   |
| Key Data   | N bytes  | []byte   | The first key in the child subtree |
| Child Hash | 32 bytes | [32]byte | SHA-256 hash of the child node     |

**Note:** The key stored in a child reference is the *first key* (minimum key) in that child's subtree. This enables binary search during tree traversal.

### Visual Example

Let's serialize an internal node with 2 children:
- Child 1: first_key="apple", hash=`0xaa...aa` (32 bytes of 0xaa)
- Child 2: first_key="banana", hash=`0xbb...bb` (32 bytes of 0xbb)

```
Byte Position:  0      1    2    3    4      5    6    7    8      9   10   11   12   13
               ┌───┬────────────────────┬──────────────────────┬──────────────────────────
               │02 │ 00   00   00   02 │ 00   00   00   05    │ 61  70  70  6C  65  
               └───┴────────────────────┴──────────────────────┴──────────────────────────
                 │           │                    │                        │
            Node Type    Child Count         Key Length              Key Data
            = 0x02       = 2                 = 5                     = "apple"
            (internal)   (big-endian)        (big-endian)            (UTF-8)


               14   15   16   17   18   19   20   21   22   23   24   25   26   27   28
              ┬────────────────────────────────────────────────────────────────────────────
              │ AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  
              ┴────────────────────────────────────────────────────────────────────────────
                                              │
                                        Child Hash (32 bytes)
                                        = 0xaaaa...aaaa
                                        (SHA-256 hash)


               29   30   31   32   33   34   35   36   37   38   39   40   41   42   43   44
              ┬────────────────────────────────────────────────────────────────────────────────
              │ AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  AA  
              ┴────────────────────────────────────────────────────────────────────────────────
                                        (hash continues)


               45     46   47   48   49     50   51   52   53   54   55
              ┬────┬──────────────────────┬────────────────────────────────
              │ AA │ 00   00   00   06    │ 62  61  6E  61  6E  61  
              ┴────┴──────────────────────┴────────────────────────────────
                │            │                        │
           (hash end)   Key Length              Key Data
                        = 6                     = "banana"
                        (big-endian)            (UTF-8)


               56   57   58   59   60   61   62   63   64   65   66   67   68   69   70   71
              ┬────────────────────────────────────────────────────────────────────────────────
              │ BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  
              ┴────────────────────────────────────────────────────────────────────────────────
                                        Child Hash (32 bytes)
                                        = 0xbbbb...bbbb


               72   73   74   75   76   77   78   79   80   81   82   83   84   85   86   87
              ┬────────────────────────────────────────────────────────────────────────────────
              │ BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  BB  
              ┴────────────────────────────────────────────────────────────────────────────────
                                    (hash continues and ends)
```

### Breakdown by Section

```
┌──────────────────────────────────────────────────────────────┐
│ SECTION 1: HEADER (5 bytes)                                  │
├──────────────────────────────────────────────────────────────┤
│ Byte 0:       0x02 (node type = internal)                    │
│ Bytes 1-4:    0x00000002 (child count = 2)                   │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ SECTION 2: FIRST CHILD REFERENCE (41 bytes)                  │
├──────────────────────────────────────────────────────────────┤
│ Bytes 5-8:    0x00000005 (key length = 5)                    │
│ Bytes 9-13:   0x6170706C65 ("apple" in hex)                  │
│ Bytes 14-45:  0xaaaa...aaaa (32-byte SHA-256 hash)           │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ SECTION 3: SECOND CHILD REFERENCE (42 bytes)                 │
├──────────────────────────────────────────────────────────────┤
│ Bytes 46-49:  0x00000006 (key length = 6)                    │
│ Bytes 50-55:  0x62616E616E61 ("banana" in hex)               │
│ Bytes 56-87:  0xbbbb...bbbb (32-byte SHA-256 hash)           │
└──────────────────────────────────────────────────────────────┘

TOTAL SIZE: 5 + 41 + 42 = 88 bytes
```

### Size Calculation

For an internal node with N children:
```
Total Size = 5 + Σ(4 + key_length[i] + 32) for i=1 to N
           = 5 + Σ(36 + key_length[i])
           = 5 + 36*N + Σ(key_length[i])
```

---

## Design Rationale

### Why Big-Endian?

Big-endian encoding ensures that the same data produces the same byte sequence regardless of the CPU architecture (x86, ARM, etc.). This is critical for:
- **Content addressing**: Same node → same hash → deduplication works
- **Cross-platform compatibility**: Files can be shared between different systems

### Why Length-Prefixed?

Length-prefixed encoding (storing length before data) allows:
- **Variable-length data**: Keys and values can be any size
- **Efficient parsing**: We know exactly how many bytes to read
- **No delimiters needed**: No need to escape special characters

### Why Type Byte?

The type byte prefix allows:
- **Generic deserialization**: `DeserializeNode()` can determine node type
- **Format versioning**: Future node types can use different prefixes
- **Validation**: Detect corrupted data early

### Why 32-byte Hashes?

SHA-256 produces 32-byte (256-bit) hashes, which provide:
- **Collision resistance**: Virtually impossible to find two different nodes with the same hash
- **Security**: Cryptographically secure
- **Standard size**: Fixed 32 bytes makes parsing simple

---

## Implementation Notes

### Serialization Process

1. Calculate total buffer size needed (avoid reallocations)
2. Allocate byte slice with exact capacity
3. Write fields in order using `binary.BigEndian.PutUint32()`
4. Return complete byte slice

### Deserialization Process

1. Read and validate node type byte
2. Read count field (pairs or children)
3. Loop through count, reading each element:
   - Read length field
   - Read data of that length
   - For internal nodes, read 32-byte hash
4. Validate that we consumed all bytes (detect truncation)

### Error Handling

Deserialization returns `ErrCorruptedData` if:
- Node type is invalid
- Not enough bytes for a length field
- Not enough bytes for data indicated by length
- Extra bytes remain after parsing

---

## Examples in Code

### Serializing a Leaf Node

```go
node := &types.LeafNode{
    Pairs: []types.KVPair{
        {Key: []byte("user"), Value: []byte("alice")},
        {Key: []byte("age"), Value: []byte("25")},
    },
}

data, err := tree.SerializeLeafNode(node)
// data is now 35 bytes as shown in the example above
```

### Deserializing a Node

```go
// Read bytes from CAS
data, err := cas.Read(hash)

// Deserialize to node
node, err := tree.DeserializeNode(data)

// Check type and use
if node.IsLeaf() {
    leafNode := node.(*types.LeafNode)
    // Access leafNode.Pairs
} else {
    internalNode := node.(*types.InternalNode)
    // Access internalNode.Children
}
```

---

## Testing

The serialization format is validated by property-based tests:

- **Property 17 (Determinism)**: Serializing the same node multiple times produces identical bytes
- **Property 18 (Round-Trip)**: Serialize → Deserialize produces an equivalent node

These tests run 100+ iterations with randomly generated nodes to ensure correctness across all possible inputs.
