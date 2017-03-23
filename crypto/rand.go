package crypto

import (
	"crypto/rand"
	"hash"
	"io"
	"math"
	"sync"
	"unsafe"
)

// A randReader produces random values via repeated hashing. The entropy field
// is the concatenation of an initial seed and a 128-bit counter. Each time
// the entropy is hashed, the counter is incremented.
type randReader struct {
	entropy [16 + HashSize]byte
	h       hash.Hash
	buf     [32]byte
	mu      sync.Mutex
}

// Read fills b with random data. It always returns len(b), nil.
func (r *randReader) Read(b []byte) (int, error) {
	r.mu.Lock()
	n := 0
	for n < len(b) {
		// Increment counter.
		*(*uint64)(unsafe.Pointer(&r.entropy[0]))++
		if *(*uint64)(unsafe.Pointer(&r.entropy[0])) == 0 {
			*(*uint64)(unsafe.Pointer(&r.entropy[8]))++
		}
		// Hash the counter + initial seed.
		r.h.Reset()
		r.h.Write(r.entropy[:])
		r.h.Sum(r.buf[:0])

		// Fill out 'b'.
		n += copy(b[n:], r.buf[:])
	}
	r.mu.Unlock()
	return n, nil
}

// Reader is a global, shared instance of a cryptographically strong pseudo-
// random generator. Reader is safe for concurrent use by multiple goroutines.
var Reader = func() *randReader {
	r := &randReader{h: NewHash()}
	// Use 64 bytes in case the first 32 aren't completely random.
	_, err := io.CopyN(r.h, rand.Reader, 64)
	if err != nil {
		panic("crypto: no entropy available")
	}
	r.h.Sum(r.entropy[16:])
	return r
}()

// Read is a helper function that calls Reader.Read on b. It always fills b
// completely.
func Read(b []byte) { Reader.Read(b) }

// Bytes is a helper function that returns n bytes of random data.
func RandBytes(n int) []byte {
	b := make([]byte, n)
	Read(b)
	return b
}

// RandIntn returns a uniform random value in [0,n). It panics if n <= 0.
func RandIntn(n int) int {
	if n <= 0 {
		panic("crypto: argument to Intn is <= 0")
	}
	// To eliminate modulo bias, keep selecting at random until we fall within
	// a range that is evenly divisible by n.
	// NOTE: since n is at most math.MaxUint64/2, max is minimized when:
	//    n = math.MaxUint64/4 + 1 -> max = math.MaxUint64 - math.MaxUint64/4
	// This gives an expected 1.333 tries before choosing a value < max.
	max := math.MaxUint64 - math.MaxUint64%uint64(n)
	b := RandBytes(8)
	r := *(*uint64)(unsafe.Pointer(&b[0]))
	for r >= max {
		Read(b)
		r = *(*uint64)(unsafe.Pointer(&b[0]))
	}
	return int(r % uint64(n))
}

// Perm returns a random permutation of the integers [0,n).
func Perm(n int) []int {
	m := make([]int, n)
	for i := 1; i < n; i++ {
		j := RandIntn(i + 1)
		m[i] = m[j]
		m[j] = i
	}
	return m
}
