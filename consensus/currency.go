package consensus

import (
	"math/big"
)

// A Currency is a 128-bit signed integer. Currency operations are performed
// via math/big.
//
// The Currency object also keeps track of whether an overflow has occurred
// during arithmetic operations. Once the 'overflow' flag has been set to
// true, it can never be reset; a new Currency must be created. Callers can
// check for overflow using the Overflow method. This allows arithmetic
// operations to be chained together without needing to check an error value
// after each operation.
type Currency struct {
	b  [16]byte
	of bool // has an overflow ever occurred?
}

func NewCurrency(x uint64) *Currency {
	return BigToCurrency(new(big.Int).SetUint64(x))
}

func BigToCurrency(b *big.Int) *Currency {
	c := new(Currency)
	copy(c.b[:], b.Bytes())
	c.of = b.BitLen() > 128
	return c
}

func (c *Currency) SetBig(b *big.Int) *Currency {
	c.b = [16]byte{}
	copy(c.b[:], b.Bytes())
	c.of = c.of || b.BitLen() > 128 // preserve overflow flag
	return c
}

func (c *Currency) Big() *big.Int {
	return new(big.Int).SetBytes(c.b[:])
}

func (c *Currency) Add(y Currency) *Currency {
	return c.SetBig(new(big.Int).Add(c.Big(), y.Big()))
}

func (c *Currency) Sub(y Currency) *Currency {
	return c.SetBig(new(big.Int).Sub(c.Big(), y.Big()))
}

func (c *Currency) Mul(y Currency) *Currency {
	return c.SetBig(new(big.Int).Mul(c.Big(), y.Big()))
}

func (c *Currency) Div(y Currency) *Currency {
	return c.SetBig(new(big.Int).Div(c.Big(), y.Big()))
}

func (c *Currency) Sign() int {
	return c.Big().Sign()
}

func (c *Currency) Cmp(y Currency) int {
	return c.Big().Cmp(y.Big())
}

// Overflow returns whether an overflow has ever occurred while setting the
// value of c. The overflow is never cleared, even if the bits of c are reset.
func (c *Currency) Overflow() bool {
	return c.of
}

// MarshalSia implements the encoding.SiaMarshaler interface. The overflow
// flag is not included; an Currency will always be encoded as 16 bytes.
func (c Currency) MarshalSia() []byte {
	return c.b[:]
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface. Ecactly 16
// bytes are consumed, and the overflow flag is not affected.
func (c *Currency) UnmarshalSia(b []byte) int {
	return copy(c.b[:], b[:16])
}
