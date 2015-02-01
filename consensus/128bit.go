package consensus

import (
	"math/big"
)

// An int128 is a 128-bit signed integer. Operations on int128s are performed
// via math/big.
//
// The int128 also keeps track of whether an overflow has occurred during
// arithmetic operations. Once the 'overflow' flag has been set to true, it
// can never be reset; a new int128 must be created. Callers can check for
// overflow using the Overflow method. This allows arithmetic operations to be
// chained together without needing to check an error value after each
// operation.
type int128 struct {
	b  [16]byte
	of bool // has an overflow ever occurred?
}

func NewInt128(x uint64) *int128 {
	return BigToInt128(new(big.Int).SetUint64(x))
}

func BigToInt128(b *big.Int) *int128 {
	i := new(int128)
	copy(i.b[:], b.Bytes())
	i.of = b.BitLen() > 128
	return i
}

func (x *int128) SetBig(b *big.Int) *int128 {
	x.b = [16]byte{}
	copy(x.b[:], b.Bytes())
	x.of = x.of || b.BitLen() > 128 // preserve overflow flag
	return x
}

func (x *int128) Big() *big.Int {
	return new(big.Int).SetBytes(x.b[:])
}

func (x *int128) Add(y *int128) *int128 {
	return x.SetBig(new(big.Int).Add(x.Big(), y.Big()))
}

func (x *int128) Sub(y *int128) *int128 {
	return x.SetBig(new(big.Int).Sub(x.Big(), y.Big()))
}

func (x *int128) Mul(y *int128) *int128 {
	return x.SetBig(new(big.Int).Mul(x.Big(), y.Big()))
}

func (x *int128) Div(y *int128) *int128 {
	return x.SetBig(new(big.Int).Div(x.Big(), y.Big()))
}

// Overflow returns whether an overflow has ever occurred while setting the
// value of x. The overflow is never cleared, even if the bits of x are reset.
func (x *int128) Overflow() bool {
	return x.of
}

// MarshalSia implements the encoding.SiaMarshaler interface. The overflow
// flag is not included; an int128 will always be encoded as 16 bytes.
func (x int128) MarshalSia() []byte {
	return x.b[:]
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface. Exactly 16
// bytes are consumed, and the overflow flag is not affected.
func (x *int128) UnmarshalSia(b []byte) int {
	return copy(x.b[:], b[:16])
}
