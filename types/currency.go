package types

// currency.go defines the internal currency object. One design goal of the
// currency type is immutability: the currency type should be safe to pass
// directly to other objects and packages. The currency object should never
// have a negative value. The currency should never overflow. There is a
// maximum size value that can be encoded (around 10^10^20), however exceeding
// this value will not result in overflow.

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/NebulousLabs/Sia/build"
)

type (
	// A Currency represents a number of siacoins or siafunds. Internally, a
	// Currency value is unbounded; however, Currency values sent over the wire
	// protocol are subject to a maximum size of 255 bytes (approximately 10^614).
	// Unlike the math/big library, whose methods modify their receiver, all
	// arithmetic Currency methods return a new value. Currency cannot be negative.
	Currency struct {
		i big.Int
	}
)

var (
	ZeroCurrency = NewCurrency64(0)

	ErrNegativeCurrency = errors.New("negative currency not allowed")
)

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

// Add returns a new Currency value c = x + y
func (x Currency) Add(y Currency) (c Currency) {
	c.i.Add(&x.i, &y.i)
	return
}

// Big returns the value of c as a *big.Int. Importantly, it does not provide
// access to the c's internal big.Int object, only a copy.
func (c Currency) Big() *big.Int {
	return new(big.Int).Set(&c.i)
}

// Cmp compares two Currency values. The return value follows the convention
// of math/big.
func (x Currency) Cmp(y Currency) int {
	return x.i.Cmp(&y.i)
}

// Div returns a new Currency value c = x / y.
func (x Currency) Div(y Currency) (c Currency) {
	c.i.Div(&x.i, &y.i)
	return
}

// Mul returns a new Currency value c = x * y.
func (x Currency) Mul(y Currency) (c Currency) {
	c.i.Mul(&x.i, &y.i)
	return
}

// COMPATv0.4.0 - until the first 10e3 blocks have been archived, MulFloat is
// needed while verifying the first set of blocks.
//
// MulFloat returns a new Currency value y = c * x, where x is a float64.
// Behavior is undefined when x is negative.
func (x Currency) MulFloat(y float64) (c Currency) {
	if y < 0 {
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		cRat := new(big.Rat).Mul(
			new(big.Rat).SetInt(&x.i),
			new(big.Rat).SetFloat64(y),
		)
		c.i.Div(cRat.Num(), cRat.Denom())
	}
	return
}

// MulRat returns a new Currency value c = x * y, where y is a big.Rat.
func (x Currency) MulRat(y *big.Rat) (c Currency) {
	if y.Sign() < 0 {
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		c.i.Mul(&x.i, y.Num())
		c.i.Div(&c.i, y.Denom())
	}
	return
}

// MulTax returns a new Currency value c = x * 0.039, where 0.039 is a big.Rat.
func (x Currency) MulTax() (c Currency) {
	c.i.Mul(&x.i, big.NewInt(39))
	c.i.Div(&c.i, big.NewInt(1000))
	return c
}

// RoundDown returns the largest multiple of y <= x.
func (x Currency) RoundDown(y Currency) (c Currency) {
	diff := new(big.Int).Mod(&x.i, &y.i)
	c.i.Sub(&x.i, diff)
	return
}

// IsZero returns true if the value is 0, false otherwise.
func (c Currency) IsZero() bool {
	return c.i.Sign() <= 0
}

// Sqrt returns a new Currency value y = sqrt(c). Result is rounded down to the
// nearest integer.
func (x Currency) Sqrt() (c Currency) {
	f, _ := new(big.Rat).SetInt(&x.i).Float64()
	sqrt := new(big.Rat).SetFloat64(math.Sqrt(f))
	c.i.Div(sqrt.Num(), sqrt.Denom())
	return
}

// Sub returns a new Currency value c = x - y. Behavior is undefined when
// x < y.
func (x Currency) Sub(y Currency) (c Currency) {
	if x.Cmp(y) < 0 {
		c = x
		if build.DEBUG {
			panic(ErrNegativeCurrency)
		}
	} else {
		c.i.Sub(&x.i, &y.i)
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
func (c *Currency) UnmarshalSia(b []byte) error {
	c.i.SetBytes(b)
	return nil
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
