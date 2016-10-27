package crypto

import (
	"crypto/rand"
	"math/big"
	"sync/atomic"
)

var (
	// Two counters is used in case the first overflows. The first is not at
	// risk of overflowing, but we're paranoid.
	counter     uint64
	counter2    uint64
	entropyBase Hash
)

// init will create an entropy pool that the RNG will draw from. On most
// systems, hashing out of a preset RNG pool will be substantially faster than
// using crypto/rand, which relies on syscalls.
func init() {
	// Use 64 bytes in case the first 32 aren't completely random.
	base := make([]byte, 64)
	_, err := rand.Read(base)
	if err != nil {
		panic("unable to set entropy for the crypto package")
	}
	entropyBase = HashObject(base)
}

// RandBytes returns n bytes of random data.
func RandBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	for i := 0; i < n; i += HashSize {
		// Fetch and update the counter values before executing the hash.
		var newCounter, newCounter2 uint64
		newCounter = atomic.AddUint64(&counter, 1)
		if newCounter == 0 {
			newCounter2 = atomic.AddUint64(&counter2, 1)
		} else {
			newCounter2 = atomic.LoadUint64(&counter2)
		}
		// Grab some entropy using the unique counter set.
		entropy := HashAll(newCounter, newCounter2, entropyBase)

		// Fill out 'b'.
		copy(b[i:], entropy[:])
	}
	return b, nil
}

// RandIntn returns a non-negative random integer in the range [0,n). It panics
// if n <= 0.
func RandIntn(n int) (int, error) {
	if n <= 0 {
		panic("RandIntn must be called with a positive, nonzero number")
	}

	// Fetch and update the counter values before executing the hash.
	var newCounter, newCounter2 uint64
	newCounter = atomic.AddUint64(&counter, 1)
	if newCounter == 0 {
		newCounter2 = atomic.AddUint64(&counter2, 1)
	} else {
		newCounter2 = atomic.LoadUint64(&counter2)
	}
	// Grab some entropy using the unique counter set.
	entropy := HashAll(newCounter, newCounter2, entropyBase)

	// Convert the first 24 bytes into a big.Int, then grab the modulus of n.
	// 24 bytes means that there is at least 16 bytes of overflow, which means
	// early numbers are favored by less than 1-in-2^128 - a cryptographically
	// safe preference.
	b := new(big.Int)
	b.SetBytes(entropy[:24])

	// Take the modules of 'b'  and 'n' and return the result as an int.
	return int(b.Mod(b, big.NewInt(int64(n))).Int64()), nil
}

// Read will fill 'b' with completely random data.
func Read(b []byte) {
	n := len(b)
	for i := 0; i < n; i += HashSize {
		// Fetch and update the counter values before executing the hash.
		var newCounter, newCounter2 uint64
		newCounter = atomic.AddUint64(&counter, 1)
		if newCounter == 0 {
			newCounter2 = atomic.AddUint64(&counter2, 1)
		} else {
			newCounter2 = atomic.LoadUint64(&counter2)
		}
		// Grab some entropy using the unique counter set.
		entropy := HashAll(newCounter, newCounter2, entropyBase)

		// Fill out 'b'.
		copy(b[i:], entropy[:])
	}
}

// Perm returns, as a slice of n ints, a random permutation of the integers
// [0,n).
func Perm(n int) ([]int, error) {
	m := make([]int, n)
	for i := 0; i < n; i++ {
		j, _ := RandIntn(i + 1)
		m[i] = m[j]
		m[j] = i
	}
	return m, nil
}
