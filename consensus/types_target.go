package consensus

import (
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

type (
	// A Target is a hash that a block's ID must be "less than" in order for
	// the block to be considered valid. Miners vary the block's 'Nonce' field
	// in order to brute-force such an ID. The inverse of a Target is called
	// the "difficulty," because it is proportional to the amount of time
	// required to brute-force the Target.
	Target crypto.Hash
)

// Int converts a Target to a big.Int.
func (t Target) Int() *big.Int {
	return new(big.Int).SetBytes(t[:])
}

// Rat converts a Target to a big.Rat.
func (t Target) Rat() *big.Rat {
	return new(big.Rat).SetInt(t.Int())
}

// Inverse returns the inverse of a Target as a big.Rat
func (t Target) Inverse() *big.Rat {
	return new(big.Rat).Inv(t.Rat())
}

// IntToTarget converts a big.Int to a Target.
func IntToTarget(i *big.Int) (t Target) {
	// i may overflow the maximum target.
	// In the event of overflow, return the maximum.
	if i.BitLen() > 256 {
		return RootDepth
	}
	b := i.Bytes()
	// need to preserve big-endianness
	offset := len(t[:]) - len(b)
	copy(t[offset:], b)
	return
}

// RatToTarget converts a big.Rat to a Target.
func RatToTarget(r *big.Rat) Target {
	// conversion to big.Int truncates decimal
	i := new(big.Int).Div(r.Num(), r.Denom())
	return IntToTarget(i)
}
