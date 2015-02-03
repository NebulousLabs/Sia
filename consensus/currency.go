package consensus

import (
	"errors"
	"math/big"
)

var (
	ErrOverflow = errors.New("Currency overflowed 128 bits")
)

// A Currency is a 128-bit unsigned integer. Currency operations are performed
// via math/big.
//
// The Currency object also keeps track of whether an overflow has occurred
// during arithmetic operations. Once the 'overflow' flag has been set to
// true, all subsequent operations will return an error, and the result of the
// operation is undefined. This flag can never be reset; a new Currency must
// be created. Callers can also manually check for overflow using the Overflow
// method.
type Currency struct {
	b  [16]byte
	of bool // has an overflow ever occurred?
}

func NewCurrency(x uint64) Currency {
	// no possibility of error
	c, _ := BigToCurrency(new(big.Int).SetUint64(x))
	return c
}

func BigToCurrency(b *big.Int) (c Currency, err error) {
	if b.BitLen() > 128 || b.Sign() < 0 {
		c.of = true
		err = ErrOverflow
		return
	}
	copy(c.b[:], b.Bytes())
	return
}

func (c *Currency) SetBig(b *big.Int) (err error) {
	y, err := BigToCurrency(b)
	c.b = y.b
	c.of = c.of || y.of
	return
}

func (c *Currency) Big() *big.Int {
	return new(big.Int).SetBytes(c.b[:])
}

func (c *Currency) Add(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(new(big.Int).Add(c.Big(), y.Big()))
}

func (c *Currency) Sub(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(new(big.Int).Sub(c.Big(), y.Big()))
}

func (c *Currency) Mul(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(new(big.Int).Mul(c.Big(), y.Big()))
}

func (c *Currency) Div(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(new(big.Int).Div(c.Big(), y.Big()))
}

func (c *Currency) Sqrt() Currency {
	f, _ := new(big.Rat).SetInt(c.Big()).Float64()
	rat := new(big.Rat).SetFloat64(f)
	// no possibility of error
	s, _ := BigToCurrency(new(big.Int).Div(rat.Num(), rat.Denom()))
	return s
}

func (c *Currency) Sign() int {
	return c.Big().Sign()
}

func (c *Currency) Cmp(y Currency) int {
	return c.Big().Cmp(y.Big())
}

func (c *Currency) Overflow() bool {
	return c.of
}
