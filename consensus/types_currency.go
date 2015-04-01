package consensus

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

var (
	ZeroCurrency = NewCurrency64(0)
)

// currency.go defines the internal currency object. One major design goal of
// the currency type is immutability. Another is non-negativity: the currency
// object should never have a negative value.

// A Currency represents a number of siacoins or siafunds. Internally, a
// Currency value is unbounded; however, Currency values sent over the wire
// protocol are subject to a maximum size of 255 bytes (approximately 10^614).
// Unlike the math/big library, whose methods modify their receiver, all
// arithmetic Currency methods return a new value.
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

// Add returns a new Currency value y = c + x.
func (c Currency) Add(x Currency) (y Currency) {
	y.i.Add(&c.i, &x.i)
	return
}

// Big returns the value of c as a *big.Int. Importantly, it does not provide
// access to the c's internal big.Int object, only a copy.
func (c Currency) Big() *big.Int {
	return new(big.Int).Set(&c.i)
}

// Cmp compares two Currency values. The return value follows the convention
// of math/big.
func (c Currency) Cmp(y Currency) int {
	return c.i.Cmp(&y.i)
}

// Div returns a new Currency value y = c / x.
func (c Currency) Div(x Currency) (y Currency) {
	y.i.Div(&c.i, &x.i)
	return
}

// Mul returns a new Currency value y = c * x.
func (c Currency) Mul(x Currency) (y Currency) {
	y.i.Mul(&c.i, &x.i)
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

// RoundDown returns the largest multiple of n <= c.
func (c Currency) RoundDown(n uint64) (y Currency) {
	diff := new(big.Int).Mod(&c.i, new(big.Int).SetUint64(n))
	y.i.Sub(&c.i, diff)
	return
}

// Sqrt returns a new Currency value y = sqrt(c)
func (c Currency) Sqrt() (y Currency) {
	f, _ := new(big.Rat).SetInt(&c.i).Float64()
	sqrt := new(big.Rat).SetFloat64(math.Sqrt(f))
	y.i.Div(sqrt.Num(), sqrt.Denom())
	return
}

// Sub returns a new Currency value y = c - x.
func (c Currency) Sub(x Currency) (y Currency) {
	y.i.Sub(&c.i, &x.i)
	return
}

// MarshalJSON implements the json.Marshaler interface.
func (c Currency) MarshalJSON() ([]byte, error) {
	return c.i.MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (c *Currency) UnmarshalJSON(b []byte) error {
	err := c.i.UnmarshalJSON(b)
	if err != nil {
		return err
	}
	if c.Cmp(ZeroCurrency) < 0 {
		return errors.New("cannot have a negative currency")
	}
	return nil
}

// MarshalSia implements the encoding.SiaMarshaler interface. It returns the
// byte-slice representation of the Currency's internal big.Int, prepended
// with a single byte indicating the length of the slice.
func (c Currency) MarshalSia() []byte {
	b := c.i.Bytes()
	if DEBUG {
		if len(b) > 255 {
			panic(len(b))
			panic("attempting to marshal a too-big currency type")
		}
	}

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

// String implements the fmt.Stringer interface.
func (c Currency) String() string {
	return c.i.String()
}

// Scan implements the fmt.Scanner interface, allowing Currency values to be
// scanned from text.
func (c *Currency) Scan(s fmt.ScanState, ch rune) error {
	err := c.i.Scan(s, ch)
	if err != nil {
		return err
	}
	if c.Cmp(ZeroCurrency) < 0 {
		return errors.New("cannot have a negative currency")
	}
	return nil
}
