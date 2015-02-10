package consensus

import (
	"math"
	"math/big"
)

// A Currency represents a number of siacoins or siafunds. Internally, a
// Currency value is unbounded; however, Currency values sent over the wire
// protocol are subject to a maximum size of 255 bytes (approximately 10^614).
// Unlike the math/big library, whose methods modify their receiver, all
// arithmetic Currency methods return a new value. This is necessary to
// preserve the immutability of types containing Currency fields.
type Currency struct {
	i big.Int
}

func NewCurrency(b *big.Int) (c Currency) {
	c.i = *b
	return
}

func NewCurrency64(x uint64) (c Currency) {
	c.i.SetUint64(x)
	return
}

func (c Currency) Big() *big.Int {
	return &c.i
}

func (c Currency) Cmp(y Currency) int {
	return c.i.Cmp(&y.i)
}

func (c Currency) IsZero() bool {
	return c.i.Sign() == 0
}

func (c Currency) Add(x Currency) (y Currency) {
	y.i.Add(&c.i, &x.i)
	return
}

func (c Currency) Sub(x Currency) (y Currency) {
	y.i.Sub(&c.i, &x.i)
	return
}

func (c Currency) Mul(x Currency) (y Currency) {
	y.i.Mul(&c.i, &x.i)
	return
}

func (c Currency) Div(x Currency) (y Currency) {
	y.i.Div(&c.i, &x.i)
	return
}

func (c Currency) MulFloat(x float64) (y Currency) {
	yRat := new(big.Rat).Mul(
		new(big.Rat).SetInt(&c.i),
		new(big.Rat).SetFloat64(x),
	)
	y.i.Div(yRat.Num(), yRat.Denom())
	return
}

func (c Currency) Sqrt() (y Currency) {
	f, _ := new(big.Rat).SetInt(&c.i).Float64()
	sqrt := new(big.Rat).SetFloat64(math.Sqrt(f))
	y.i.Div(sqrt.Num(), sqrt.Denom())
	return
}

// RoundDown returns the largest multiple of n <= c.
func (c Currency) RoundDown(n uint64) (y Currency) {
	diff := new(big.Int).Mod(&c.i, new(big.Int).SetUint64(n))
	y.i.Sub(&c.i, diff)
	return
}

// MarshalSia implements the encoding.SiaMarshaler interface. It returns the
// byte-slice representation of the Currency's internal big.Int, prepended
// with a single byte indicating the length of the slice. This implies a
// maximum encodable value of 2^(255 * 8), or approximately 10^614.
//
// Note that as the bytes of the big.Int correspond to the absolute value of
// the integer, there is no way to marshal a negative Currency.
func (c Currency) MarshalSia() []byte {
	b := c.i.Bytes()
	return append(
		[]byte{byte(len(b))},
		b...,
	)
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface. See
// MarshalSia for a description of how Currency values are marshalled.
func (c *Currency) UnmarshalSia(b []byte) int {
	var n int
	n, b = int(b[0]), b[1:]
	c.i.SetBytes(b[:n])
	return 1 + n
}
