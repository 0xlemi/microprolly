package chunker

// Buzhash implements a rolling hash algorithm for content-defined chunking.
// It uses a table of random values to compute a hash over a sliding window.
type Buzhash struct {
	// TargetSize is the average chunk size in bytes (boundary when hash % targetSize == 0)
	TargetSize uint32
	// MinSize prevents tiny chunks (in bytes)
	MinSize uint32
	// MaxSize prevents huge chunks (in bytes)
	MaxSize uint32

	// Internal state
	hash        uint32
	window      []byte
	pos         int
	count       int  // number of bytes processed since last reset
	boundaryHit bool // true if hash % targetSize == 0 was hit during any roll
}

// buzhashTable contains random values for each byte value (0-255)
// These values are used to compute the rolling hash
var buzhashTable = [256]uint32{
	0x458be752, 0xc10748cc, 0xfbbcdbb8, 0x6ded5b68,
	0xb10a82b5, 0x20d75648, 0xdfc5665f, 0xa8428801,
	0x7ebf5191, 0x841135c7, 0x65cc53b3, 0x280a597c,
	0x16f60255, 0xc78cbc3e, 0x294415f5, 0xb938d494,
	0xec85c4e6, 0xb7d33edc, 0xe549b544, 0xfdeda5aa,
	0x882bf287, 0x3116571e, 0xa6fc8d2d, 0x1b5f3f3c,
	0x2e7d4e29, 0x49e95d76, 0x540d0a26, 0xf87b1a02,
	0x84b4a028, 0xd7f89c1e, 0xf309cbe0, 0x600a2f4f,
	0x5f33e848, 0xb149a5d5, 0x1e39e8bd, 0x2a1fc67a,
	0x934d46e4, 0x8f902f30, 0xfc4b0223, 0xfb6d4314,
	0x5f6b9b30, 0x6f2d9c6c, 0x58597e40, 0x3cbbb848,
	0x7c3b5360, 0x3f0ab26c, 0x9ea521c8, 0x1c1b0d14,
	0x3e9de0c0, 0x289d8f1c, 0x0c01f56c, 0x61bd8e3c,
	0xd6e2e980, 0x9c098894, 0x9e0e2534, 0x049dc09c,
	0x64a0dc24, 0xb07c0440, 0x8e5b0a50, 0xf05c1e10,
	0x4c449e3c, 0x5c8c6c30, 0x88507800, 0x08b09a40,
}

// DefaultWindowSize is the size of the sliding window for the rolling hash
const DefaultWindowSize = 64

// NewBuzhash creates a new Buzhash rolling hash with the given parameters
func NewBuzhash(targetSize, minSize, maxSize uint32) *Buzhash {
	return &Buzhash{
		TargetSize:  targetSize,
		MinSize:     minSize,
		MaxSize:     maxSize,
		window:      make([]byte, DefaultWindowSize),
		hash:        0,
		pos:         0,
		count:       0,
		boundaryHit: false,
	}
}

// Reset resets the rolling hash state
func (b *Buzhash) Reset() {
	b.hash = 0
	b.pos = 0
	b.count = 0
	b.boundaryHit = false
	for i := range b.window {
		b.window[i] = 0
	}
}

// Roll updates the hash with a new byte and returns the current hash value.
// If the hash indicates a boundary (hash % targetSize == 0), it remembers this
// so that IsBoundary() will return true (respecting min/max constraints).
func (b *Buzhash) Roll(newByte byte) uint32 {
	windowSize := len(b.window)

	// Get the byte that's leaving the window
	outByte := b.window[b.pos]

	// Update the window
	b.window[b.pos] = newByte
	b.pos = (b.pos + 1) % windowSize

	// Update the hash using the rolling property:
	// hash = rotateLeft(hash, 1) ^ rotateLeft(table[outByte], windowSize) ^ table[newByte]
	b.hash = rotateLeft(b.hash, 1) ^ rotateLeft(buzhashTable[outByte], uint32(windowSize)) ^ buzhashTable[newByte]

	b.count++

	// Check if this byte triggers a boundary (only after min size)
	// We remember if ANY roll hit a boundary, so we can report it in IsBoundary()
	if b.count >= int(b.MinSize) && b.hash%b.TargetSize == 0 {
		b.boundaryHit = true
	}

	return b.hash
}

// IsBoundary checks if the current position should be a chunk boundary
// based on whether any roll hit a boundary condition and size constraints.
// All units are in bytes for consistency.
func (b *Buzhash) IsBoundary() bool {
	// Don't create boundary if we haven't reached minimum size
	if b.count < int(b.MinSize) {
		return false
	}

	// Force boundary if we've reached maximum size
	if b.count >= int(b.MaxSize) {
		return true
	}

	// Return true if any roll since last reset hit a boundary
	return b.boundaryHit
}

// Count returns the number of bytes processed since last reset
func (b *Buzhash) Count() int {
	return b.count
}

// rotateLeft performs a left rotation on a 32-bit value
func rotateLeft(val uint32, n uint32) uint32 {
	n = n % 32
	return (val << n) | (val >> (32 - n))
}
