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

// NewCurrency creates a Currency value from a big.Int.
func NewCurrency(b *big.Int) (c Currency) {
	c.i = *b
	return
}

// NewCurrency64 creates a Currency value from a uint64.
func NewCurrency64(x uint64) (c Currency) {
	c.i.SetUint64(x)
	return
}

// Big returns the value of c as a *big.Int. Importantly, it does not provide
// access to the c's internal big.Int object, only a copy. This is in
// accordance with the immutability constraint described above.
func (c Currency) Big() *big.Int {
	return new(big.Int).Set(&c.i)
}

// Cmp compares two Currency values. The return value follows the convention
// of the math/big package.
func (c Currency) Cmp(y Currency) int {
	return c.i.Cmp(&y.i)
}

// Sign returns the sign of a Currency. The return value follows of the
// convention of the math/big package.
func (c Currency) Sign() int {
	return c.i.Sign()
}

// Add returns a new Currency value y = c + x.
func (c Currency) Add(x Currency) (y Currency) {
	y.i.Add(&c.i, &x.i)
	return
}

// Sub returns a new Currency value y = c - x.
func (c Currency) Sub(x Currency) (y Currency) {
	y.i.Sub(&c.i, &x.i)
	return
}

// Mult returns a new Currency value y = c * x.
func (c Currency) Mul(x Currency) (y Currency) {
	y.i.Mul(&c.i, &x.i)
	return
}

// Div returns a new Currency value y = c / x.
func (c Currency) Div(x Currency) (y Currency) {
	y.i.Div(&c.i, &x.i)
	return
}

// MulFloat returns a new Currency value y = c * x, where x is a float64.
func (c Currency) MulFloat(x float64) (y Currency) {
	yRat := new(big.Rat).Mul(
		new(big.Rat).SetInt(&c.i),
		new(big.Rat).SetFloat64(x),
	)
	y.i.Div(yRat.Num(), yRat.Denom())
	return
}

// Sqrt returns a new Currency value y = sqrt(c)
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

// ContractTax returns the amount of Currency that would be taxed from a
// contract payout with value c.
func (c Currency) ContractTax() Currency {
	return c.MulFloat(SiafundPortion).RoundDown(SiafundCount)
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

// MarshalJSON implements the json.Marshaler interface.
func (c Currency) MarshalJSON() ([]byte, error) {
	return c.i.MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (c *Currency) UnmarshalJSON(b []byte) error {
	return c.i.UnmarshalJSON(b)
}
