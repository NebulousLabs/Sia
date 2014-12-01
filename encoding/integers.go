package encoding

import (
	"encoding/binary"
)

// EncInt64 encodes an int64 as a slice of 8 bytes.
func EncInt64(i int64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(i))
	return
}

// DecInt64 decodes a slice of 8 bytes into an int64.
// If len(b) < 8, the slice is padded with zeros.
func DecInt64(b []byte) int64 {
	b2 := b
	if len(b) < 8 {
		b2 = make([]byte, 8)
		copy(b2, b)
	}
	return int64(binary.LittleEndian.Uint64(b2))
}

// EncUint64 encodes a uint64 as a slice of 8 bytes.
func EncUint64(i uint64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, i)
	return
}

// DecUint64 decodes a slice of 8 bytes into a uint64.
// If len(b) < 8, the slice is padded with zeros.
func DecUint64(b []byte) uint64 {
	b2 := b
	if len(b) < 8 {
		b2 = make([]byte, 8)
		copy(b2, b)
	}
	return binary.LittleEndian.Uint64(b2)
}

// EncLen encodes a length (int) as a slice of 4 bytes.
func EncLen(length int) (b []byte) {
	b = make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(length))
	return
}

// DecLen decodes a slice of 4 bytes into an int.
// If len(b) < 8, the slice is padded with zeros.
func DecLen(b []byte) int {
	b2 := b
	if len(b) < 4 {
		b2 = make([]byte, 4)
		copy(b2, b)
	}
	return int(binary.LittleEndian.Uint32(b2))
}
