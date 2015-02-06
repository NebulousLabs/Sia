package consensus

import (
	"bytes"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

// CheckTarget returns true if the block id is lower than the target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// Int returns a Target as a big.Int.
func (t Target) Int() *big.Int {
	return new(big.Int).SetBytes(t[:])
}

// Rat returns a Target as a big.Rat.
func (t Target) Rat() *big.Rat {
	return new(big.Rat).SetInt(t.Int())
}

// Inv returns the inverse of a Target as a big.Rat
func (t Target) Inverse() *big.Rat {
	r := t.Rat()
	return r.Inv(r)
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
	offset := crypto.HashSize - len(b)
	copy(t[offset:], b)
	return
}

// RatToTarget converts a big.Rat to a Target.
func RatToTarget(r *big.Rat) Target {
	// convert to big.Int to truncate decimal
	i := new(big.Int).Div(r.Num(), r.Denom())
	return IntToTarget(i)
}
