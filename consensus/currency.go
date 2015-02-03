package consensus

import (
	"errors"
	"math/big"
)

func NewCurrency(x uint64) Currency {
	return BigToCurrency(new(big.Int).SetUint64(x))
}

func BigToCurrency(b *big.Int) (c Currency, err error) {
	if b.BitLen() > 128 {
		err = errors.New("overflow")
		return
	}
	if b.Sign() < 0 {
		err = errors.New("negative Currency")
		return
	}
	copy(c[:], b.Bytes())
	return
}

func (c *Currency) Big() *big.Int {
	return new(big.Int).SetBytes(c[:])
}

func (c *Currency) Sqrt() Currency {
	f, _ := new(big.Rat).SetInt(c.Big()).Float64()
	rat := new(big.Rat).SetFloat64(f)
	// no possibility of error
	s, _ := BigToCurrency(new(big.Int).Div(rat.Num(), rat.Denom()))
	return s
}
