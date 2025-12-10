package chunker

import (
	"encoding/binary"
	"errors"
	"fmt"

	"microprolly/pkg/types"
)

var (
	// ErrCorruptedData is returned when deserialization fails
	ErrCorruptedData = errors.New("data corruption detected")
)

// SerializeKVPair serializes a single KVPair to bytes using deterministic binary encoding.
// Format:
//
//	[4 bytes: key length (big-endian)]
//	[N bytes: key]
//	[4 bytes: value length (big-endian)]
//	[M bytes: value]
func SerializeKVPair(pair types.KVPair) []byte {
	size := 4 + len(pair.Key) + 4 + len(pair.Value)
	buf := make([]byte, 0, size)

	// Write key length
	keyLen := make([]byte, 4)
	binary.BigEndian.PutUint32(keyLen, uint32(len(pair.Key)))
	buf = append(buf, keyLen...)

	// Write key
	buf = append(buf, pair.Key...)

	// Write value length
	valueLen := make([]byte, 4)
	binary.BigEndian.PutUint32(valueLen, uint32(len(pair.Value)))
	buf = append(buf, valueLen...)

	// Write value
	buf = append(buf, pair.Value...)

	return buf
}

// SerializeKVPairs serializes multiple KVPairs to bytes using deterministic binary encoding.
// Format:
//
//	[4 bytes: pair count (big-endian)]
//	For each pair:
//	  [4 bytes: key length]
//	  [N bytes: key]
//	  [4 bytes: value length]
//	  [M bytes: value]
func SerializeKVPairs(pairs []types.KVPair) []byte {
	// Calculate total size
	size := 4 // pair count
	for _, pair := range pairs {
		size += 4 + len(pair.Key) + 4 + len(pair.Value)
	}

	buf := make([]byte, 0, size)

	// Write pair count
	pairCount := make([]byte, 4)
	binary.BigEndian.PutUint32(pairCount, uint32(len(pairs)))
	buf = append(buf, pairCount...)

	// Write each pair
	for _, pair := range pairs {
		buf = append(buf, SerializeKVPair(pair)...)
	}

	return buf
}

// DeserializeKVPair deserializes bytes into a single KVPair.
// Returns the pair and the number of bytes consumed.
func DeserializeKVPair(data []byte) (types.KVPair, int, error) {
	pos := 0

	// Read key length
	if pos+4 > len(data) {
		return types.KVPair{}, 0, fmt.Errorf("%w: insufficient data for key length", ErrCorruptedData)
	}
	keyLen := binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Read key
	if pos+int(keyLen) > len(data) {
		return types.KVPair{}, 0, fmt.Errorf("%w: insufficient data for key", ErrCorruptedData)
	}
	key := make([]byte, keyLen)
	copy(key, data[pos:pos+int(keyLen)])
	pos += int(keyLen)

	// Read value length
	if pos+4 > len(data) {
		return types.KVPair{}, 0, fmt.Errorf("%w: insufficient data for value length", ErrCorruptedData)
	}
	valueLen := binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Read value
	if pos+int(valueLen) > len(data) {
		return types.KVPair{}, 0, fmt.Errorf("%w: insufficient data for value", ErrCorruptedData)
	}
	value := make([]byte, valueLen)
	copy(value, data[pos:pos+int(valueLen)])
	pos += int(valueLen)

	return types.KVPair{Key: key, Value: value}, pos, nil
}

// DeserializeKVPairs deserializes bytes into multiple KVPairs.
func DeserializeKVPairs(data []byte) ([]types.KVPair, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: insufficient data for pair count", ErrCorruptedData)
	}

	pos := 0

	// Read pair count
	pairCount := binary.BigEndian.Uint32(data[pos : pos+4])
	pos += 4

	// Read pairs
	pairs := make([]types.KVPair, 0, pairCount)
	for i := uint32(0); i < pairCount; i++ {
		pair, consumed, err := DeserializeKVPair(data[pos:])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
		pos += consumed
	}

	// Validate that we consumed all bytes
	if pos != len(data) {
		return nil, fmt.Errorf("%w: unexpected trailing data (%d bytes remaining)", ErrCorruptedData, len(data)-pos)
	}

	return pairs, nil
}
