package types

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/NebulousLabs/Sia/build"
)

var (
	ZeroCurrency = NewCurrency64(0)

	ErrNegativeCurrency = errors.New("negative currency not allowed")
)

// currency.go defines the internal currency object. One design goal of the
// currency type is immutability: the currency type should be safe to pass
// directly to other objects and packages. The currency object should never
// have a negative value. The currency should never overflow. There is a
// maximum size value that can be encoded (around 10^10^20), however exceeding
// this value will not result in overflow.

// A Currency represents a number of siacoins or siafunds. Internally, a
// Currency value is unbounded; however, Currency values sent over the wire
// protocol are subject to a maximum size of 255 bytes (approximately 10^614).
// Unlike the math/big library, whose methods modify their receiver, all
// arithmetic Currency methods return a new value. Currency cannot be negative.
type Currency struct {
	i big.Int
}

// NewCurrency creates a Currency value from a big.Int. Undefined behavior
// occurs if a negative input is used.
func NewCurrency(b *big.Int) (c Currency) {
	if b.Sign() < 0 {
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		c.i = *b
	}
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
// Behavior is undefined when x is negative.
func (c Currency) MulFloat(x float64) (y Currency) {
	if x < 0 {
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		yRat := new(big.Rat).Mul(
			new(big.Rat).SetInt(&c.i),
			new(big.Rat).SetFloat64(x),
		)
		y.i.Div(yRat.Num(), yRat.Denom())
	}
	return
}

// RoundDown returns the largest multiple of n <= c.
func (c Currency) RoundDown(n uint64) (y Currency) {
	diff := new(big.Int).Mod(&c.i, new(big.Int).SetUint64(n))
	y.i.Sub(&c.i, diff)
	return
}

// IsZero returns true if the value is 0, false otherwise.
func (c Currency) IsZero() bool {
	return c.i.Sign() <= 0
}

// Sqrt returns a new Currency value y = sqrt(c). Result is rounded down to the
// nearest integer.
func (c Currency) Sqrt() (y Currency) {
	f, _ := new(big.Rat).SetInt(&c.i).Float64()
	sqrt := new(big.Rat).SetFloat64(math.Sqrt(f))
	y.i.Div(sqrt.Num(), sqrt.Denom())
	return
}

// Sub returns a new Currency value y = c - x. Behavior is undefined when
// c < x.
func (c Currency) Sub(x Currency) (y Currency) {
	if c.Cmp(x) < 0 {
		y = c
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		y.i.Sub(&c.i, &x.i)
	}
	return
}

// MarshalJSON implements the json.Marshaler interface.
func (c Currency) MarshalJSON() ([]byte, error) {
	return c.i.MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface. An error is
// returned if a negative number is provided.
func (c *Currency) UnmarshalJSON(b []byte) error {
	err := c.i.UnmarshalJSON(b)
	if err != nil {
		return err
	}
	if c.i.Sign() < 0 {
		c.i = *big.NewInt(0)
		return ErrNegativeCurrency
	}
	return nil
}

// MarshalSia implements the encoding.SiaMarshaler interface. It returns the
// byte-slice representation of the Currency's internal big.Int.  Note that as
// the bytes of the big.Int correspond to the absolute value of the integer,
// there is no way to marshal a negative Currency.
func (c Currency) MarshalSia() []byte {
	return c.i.Bytes()
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface.
func (c *Currency) UnmarshalSia(b []byte) {
	c.i.SetBytes(b)
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
	if c.i.Sign() < 0 {
		return ErrNegativeCurrency
	}
	return nil
}
