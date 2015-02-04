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
	i  *big.Int
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
	// don't reuse b's memory.
	// this is probably excessive, and could be optimized away later.
	c.i = new(big.Int).SetBytes(b.Bytes())
	return
}

func (c *Currency) SetBig(b *big.Int) (err error) {
	oldOF := c.of
	*c, err = BigToCurrency(b)
	c.of = c.of || oldOF // preserve overflow flag
	return
}

func (c *Currency) Big() *big.Int {
	return c.i
}

func (c *Currency) Add(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(c.i.Add(c.i, y.i))
}

func (c *Currency) Sub(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(c.i.Sub(c.i, y.i))
}

func (c *Currency) Mul(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(c.i.Mul(c.i, y.i))
}

func (c *Currency) Div(y Currency) error {
	if c.of {
		return ErrOverflow
	}
	return c.SetBig(c.i.Div(c.i, y.i))
}

func (c *Currency) Sqrt() Currency {
	f, _ := new(big.Rat).SetInt(c.i).Float64()
	rat := new(big.Rat).SetFloat64(f)
	s, _ := BigToCurrency(new(big.Int).Div(rat.Num(), rat.Denom()))
	s.of = c.of // preserve overflow
	return s
}

func (c *Currency) Sign() int {
	return c.i.Sign()
}

func (c *Currency) Cmp(y Currency) int {
	return c.i.Cmp(y.i)
}

func (c *Currency) Overflow() bool {
	return c.of
}

func (c Currency) MarshalSia() []byte {
	b := make([]byte, 16)
	copy(b, c.i.Bytes())
	return b
}

func (c *Currency) UnmarshalSia(b []byte) int {
	c.i.SetBytes(b[:16])
	if c.Overflow() {
		return -1
	}
	return 16
}
