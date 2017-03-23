package crypto

import (
	"math/big"

	"github.com/NebulousLabs/fastrand"
)

// Reader is a global, shared instance of a cryptographically strong pseudo-
// random generator. Reader is safe for concurrent use by multiple goroutines.
var Reader = fastrand.Reader

// Read is a helper function that calls Reader.Read on b. It always fills b
// completely.
func Read(b []byte) { fastrand.Read(b) }

// Bytes is a helper function that returns n bytes of random data.
func RandBytes(n int) []byte { return fastrand.Bytes(n) }

// RandIntn returns a uniform random value in [0,n). It panics if n <= 0.
func RandIntn(n int) int {
	if n <= 0 {
		panic("crypto: argument to Intn is <= 0")
	}
	return fastrand.Intn(n)
}

// RandBigIntn returns a uniform random value in [0,n). It panics if n <= 0.
func RandBigIntn(n *big.Int) *big.Int { return fastrand.BigIntn(n) }

// Perm returns a random permutation of the integers [0,n).
func Perm(n int) []int { return fastrand.Perm(n) }
