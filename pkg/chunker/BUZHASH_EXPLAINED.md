# Buzhash Rolling Hash - Visual Explanation

## What is a Rolling Hash?

A rolling hash computes a hash over a **sliding window** of data. As new bytes come in, it efficiently updates the hash without recomputing from scratch.

## The Sliding Window

Imagine a window of size 4 sliding over your data:

```
Data stream: [A][B][C][D][E][F][G][H]...

Step 1: Window = [A][B][C][D]  → hash₁
             ↑___________↑

Step 2: Window = [B][C][D][E]  → hash₂ (A leaves, E enters)
                ↑___________↑

Step 3: Window = [C][D][E][F]  → hash₃ (B leaves, F enters)
                   ↑___________↑
```

The key insight: **hash₂ can be computed from hash₁** without looking at B, C, D again!

## The Buzhash Formula

```
new_hash = rotateLeft(old_hash, 1) 
         ^ rotateLeft(table[outByte], windowSize) 
         ^ table[newByte]
```

Let's break this down:

### 1. The Lookup Table

Each byte (0-255) maps to a random 32-bit number:

```
table['A'] = 0x458be752
table['B'] = 0xc10748cc
table['C'] = 0xfbbcdbb8
...
```

These are pre-generated random values. The randomness ensures different content produces different hashes.

### 2. Rotate Left

`rotateLeft(value, n)` shifts bits left, wrapping around:

```
Example: rotateLeft(0b11010010, 3)

Before:  1 1 0 1 0 0 1 0
         ↑_↑_↑           (these 3 bits wrap around)
         
After:   0 1 0 0 1 0 1 1 0
                     ↑_↑_↑ (wrapped to the right)
```

In 32-bit:
```
rotateLeft(0x80000001, 1) = 0x00000003
         1000...0001      → 0000...0011
```

### 3. Why Rotation?

Rotation encodes **position** into the hash. The same byte at different positions contributes differently:

```
Window: [A][B][C][D]

A's contribution: rotateLeft(table['A'], 3)  ← rotated 3 times (oldest)
B's contribution: rotateLeft(table['B'], 2)  ← rotated 2 times
C's contribution: rotateLeft(table['C'], 1)  ← rotated 1 time
D's contribution: table['D']                  ← no rotation (newest)
```

### 4. XOR (^) Combines Everything

XOR has a special property: `X ^ X = 0`

This lets us **remove** the outgoing byte:

```
If hash contains: ... ^ rotateLeft(table['A'], windowSize) ^ ...

We can remove A by XORing again:
hash ^ rotateLeft(table['A'], windowSize) 
    = ... ^ 0 ^ ...  ← A's contribution cancels out!
```

## Visual Example: Rolling the Window

```
Window size = 4
Data: [A][B][C][D][E]

═══════════════════════════════════════════════════════════════

INITIAL STATE (after processing A, B, C, D):

Window buffer: [A][B][C][D]
               pos=0

hash = table[A]⊕rot(table[B],1)⊕rot(table[C],2)⊕rot(table[D],3)
     = 0x458be752 ^ 0x820e9198 ^ 0xeef36ee2 ^ 0x36f6ab40
     = (some value)

═══════════════════════════════════════════════════════════════

ROLL IN 'E' (A leaves, E enters):

Step 1: outByte = window[pos] = 'A'
Step 2: window[pos] = 'E'  → Window buffer: [E][B][C][D]
Step 3: pos = (0 + 1) % 4 = 1

Step 4: Calculate new hash:
        
        old_hash           = hash of [A][B][C][D]
        
        rotateLeft(old_hash, 1)
            → Shifts all contributions left by 1
            → A was at position 3, now would be at position 4 (out of window!)
        
        ^ rotateLeft(table['A'], 4)
            → Removes A's contribution (XOR cancels it out)
        
        ^ table['E']
            → Adds E's contribution at position 0 (newest)
        
        = hash of [B][C][D][E]  ✓

═══════════════════════════════════════════════════════════════
```

## Why This Works for Chunking

The hash value is **deterministic** based on window content:

```
Same window content → Same hash → Same boundary decision

[A][B][C][D] → hash = 0x12345678
[A][B][C][D] → hash = 0x12345678  (always!)
```

And it's **content-defined**, not position-defined:

```
Data 1: [X][Y][A][B][C][D][Z]
                ↑_______↑ window here → hash = H

Data 2: [A][B][C][D][W]
        ↑_______↑ window here → hash = H (same!)
```

This is why inserting data only affects boundaries near the insertion point!

## Boundary Detection

```go
if hash % targetSize == 0 {
    boundaryHit = true
}
```

Since hash values are pseudo-random, `hash % 4096 == 0` happens roughly 1 in 4096 times.

```
Roll byte 1: hash = 0x12847293  → 0x12847293 % 4096 = 659   ✗
Roll byte 2: hash = 0x98234100  → 0x98234100 % 4096 = 256   ✗
Roll byte 3: hash = 0x45671000  → 0x45671000 % 4096 = 0     ✓ BOUNDARY!
Roll byte 4: hash = 0x73829abc  → 0x73829abc % 4096 = 2748  ✗
```

## Summary

```
┌─────────────────────────────────────────────────────────────┐
│                     BUZHASH ROLLING HASH                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Sliding window of N bytes                               │
│                                                             │
│  2. Each byte maps to random 32-bit value via lookup table  │
│                                                             │
│  3. Rotation encodes position (older bytes rotated more)    │
│                                                             │
│  4. XOR combines contributions (and allows removal)         │
│                                                             │
│  5. New hash = rotate(old) ^ remove(out) ^ add(in)         │
│                                                             │
│  6. O(1) per byte - no need to rehash entire window!       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

The magic is that we can update the hash in **constant time** regardless of window size, while still having the hash represent the entire window content.
