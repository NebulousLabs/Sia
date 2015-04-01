package consensus

import (
	"bytes"
	"math/big"
	"testing"
)

// TestCurrencyToBig tests the Big method for the currency type
func TestCurrencyToBig(t *testing.T) {
	c := NewCurrency64(125)
	cb := c.Big()
	b := big.NewInt(125)

	if b.Cmp(cb) != 0 {
		t.Error("currency to big has failed")
	}
}

// TestCurrencySqrt checks that the sqrt function of the currency type has been
// correctly implemented.
func TestCurrencySqrt(t *testing.T) {
	c8 := NewCurrency64(8)
	c64 := NewCurrency64(64)
	c80 := NewCurrency64(80)
	sqrt64 := c64.Sqrt()
	sqrt80 := c80.Sqrt()

	if c8.Cmp(sqrt64) != 0 {
		t.Error("square root of 64 should be 8")
	}
	if c8.Cmp(sqrt80) != 0 {
		t.Error("square root of 80 should be 8")
	}
}

// TestMarshalJSON probes the MarshalJSON and UnmarshalJSON functions.
func TestMarshalJSON(t *testing.T) {
	b30 := big.NewInt(30)
	c30 := NewCurrency64(30)

	bMar30, err := b30.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	cMar30, err := c30.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(bMar30, cMar30) != 0 {
		t.Error("Currency does not match the marshalling of its math/big equivalent")
	}

	var cUmar30 Currency
	err = cUmar30.UnmarshalJSON(cMar30)
	if err != nil {
		t.Fatal(err)
	}
	if c30.Cmp(cUmar30) != 0 {
		t.Error("Incorrect unmarshalling of currency type.")
	}
}

// TestMarshalSia probes the MarshalSia and UnmarshalSia functions.
func TestMarshalSia(t *testing.T) {
	c := NewCurrency64(1656)
	cMar := c.MarshalSia()
	var cUmar Currency
	cUmar.UnmarshalSia(cMar)
	if c.Cmp(cUmar) != 0 {
		t.Error("marshal and unmarshal mismatch for currency type")
	}
}

// TestNegativeCurrencyMulFloat checks that negative numbers are rejected when
// calling MulFloat on the currency type.
func TestNegativeCurrencyMulFloat(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occured when trying to create a negative currency")
		}
	}()

	c := NewCurrency64(12)
	_ = c.MulFloat(-1)
}

// TestNegativeCurrencySub checks that negative numbers are prevented when
// using subtraction on the currency type.
func TestNegativeCurrencySub(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occured when trying to create a negative currency")
		}
	}()

	c1 := NewCurrency64(1)
	c2 := NewCurrency64(2)
	_ = c1.Sub(c2)
}

// TestNegativeCurrencyUnmarshalJSON tries to unmarshal a negative number from
// JSON.
func TestNegativeCurrencyUnmarshalJSON(t *testing.T) {
	// Marshal a 2 digit number.
	c := NewCurrency64(35)
	cMar, err := c.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	// Change the first digit to a negative character.
	cMar[0] = 45

	// Try unmarshalling the negative currency.
	var cNeg Currency
	err = cNeg.UnmarshalJSON(cMar)
	if err != ErrNegativeCurrency {
		t.Error("expecting ErrNegativeCurrency:", err)
	}
	if cNeg.Cmp(ZeroCurrency) < 0 {
		t.Error("negative currency returned")
	}
}

// TestNegativeCurrencies tries an array of ways to produce a negative currency.
func TestNegativeNewCurrency(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occured when trying to create a negative currency")
		}
	}()

	// Try to create a new currency from a negative number.
	negBig := big.NewInt(-1)
	_ = NewCurrency(negBig)
}
