package types

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
)

// TestNewCurrency initializes a standard new currency.
func TestNewCurrency(t *testing.T) {
	b := big.NewInt(481)
	c := NewCurrency(b)
	if b.String() != c.String() {
		t.Error("NewCurrency does't seem to work properly")
	}
}

// TestCurrencyAdd probes the addition function of the currency type.
func TestCurrencyAdd(t *testing.T) {
	c7 := NewCurrency64(7)
	c12 := NewCurrency64(12)
	c19 := NewCurrency64(19)

	if c7.Add(c12).Cmp(c19) != 0 {
		t.Error("Add doesn't seem to work right")
	}
}

// TestCurrencyToBig tests the Big method for the currency type
func TestCurrencyToBig(t *testing.T) {
	c := NewCurrency64(125)
	cb := c.Big()
	b := big.NewInt(125)

	if b.Cmp(cb) != 0 {
		t.Error("currency to big has failed")
	}
}

// TestCurrencyCmp tests the Cmp method for the currency type
func TestCurrencyCmp(t *testing.T) {
	tests := []struct {
		x, y Currency
		exp  int
	}{
		{NewCurrency64(0), NewCurrency64(0), 0},
		{NewCurrency64(0), NewCurrency64(1), -1},
		{NewCurrency64(1), NewCurrency64(0), 1},
		{NewCurrency64(100), NewCurrency64(7), 1},
		{NewCurrency64(777), NewCurrency(big.NewInt(777)), 0},
		{NewCurrency(big.NewInt(7)), NewCurrency(big.NewInt(8)), -1},
	}

	for _, test := range tests {
		if c := test.x.Cmp(test.y); c != test.exp {
			t.Errorf("expected %v.Cmp(%v) == %v, got %v", test.x, test.y, test.exp, c)
		} else if bc := test.x.Big().Cmp(test.y.Big()); c != bc {
			t.Errorf("Currency.Cmp (%v) does not match big.Int.Cmp (%v) for %v.Cmp(%v)", c, bc, test.x, test.y)
		}
	}
}

// TestCurrencyCmp64 tests the Cmp64 method for the currency type
func TestCurrencyCmp64(t *testing.T) {
	tests := []struct {
		x   Currency
		y   uint64
		exp int
	}{
		{NewCurrency64(0), 0, 0},
		{NewCurrency64(0), 1, -1},
		{NewCurrency64(1), 0, 1},
		{NewCurrency64(100), 7, 1},
		{NewCurrency64(777), 777, 0},
		{NewCurrency(big.NewInt(7)), 8, -1},
	}

	for _, test := range tests {
		if c := test.x.Cmp64(test.y); c != test.exp {
			t.Errorf("expected %v.Cmp64(%v) == %v, got %v", test.x, test.y, test.exp, c)
		} else if bc := test.x.Big().Cmp(big.NewInt(int64(test.y))); c != bc {
			t.Errorf("Currency.Cmp64 (%v) does not match big.Int.Cmp (%v) for %v.Cmp64(%v)", c, bc, test.x, test.y)
		}
	}
}

// TestCurrencyDiv checks that the div function has been correctly implemented.
func TestCurrencyDiv(t *testing.T) {
	c9 := NewCurrency64(9)
	c10 := NewCurrency64(10)
	c90 := NewCurrency64(90)
	c97 := NewCurrency64(97)

	c90D10 := c90.Div(c10)
	if c90D10.Cmp(c9) != 0 {
		t.Error("Dividing 90 by 10 should produce 9")
	}
	c97D10 := c97.Div(c10)
	if c97D10.Cmp(c9) != 0 {
		t.Error("Dividing 97 by 10 should produce 9")
	}
}

// TestCurrencyDiv64 checks that the Div64 function has been correctly implemented.
func TestCurrencyDiv64(t *testing.T) {
	c9 := NewCurrency64(9)
	u10 := uint64(10)
	c90 := NewCurrency64(90)
	c97 := NewCurrency64(97)

	c90D10 := c90.Div64(u10)
	if c90D10.Cmp(c9) != 0 {
		t.Error("Dividing 90 by 10 should produce 9")
	}
	c97D10 := c97.Div64(u10)
	if c97D10.Cmp(c9) != 0 {
		t.Error("Dividing 97 by 10 should produce 9")
	}
}

// TestCurrencyEquals tests the Equals method for the currency type
func TestCurrencyEquals(t *testing.T) {
	tests := []struct {
		x, y Currency
		exp  bool
	}{
		{NewCurrency64(0), NewCurrency64(0), true},
		{NewCurrency64(0), NewCurrency64(1), false},
		{NewCurrency64(1), NewCurrency64(0), false},
		{NewCurrency64(100), NewCurrency64(7), false},
		{NewCurrency64(777), NewCurrency(big.NewInt(777)), true},
		{NewCurrency(big.NewInt(7)), NewCurrency(big.NewInt(8)), false},
	}

	for _, test := range tests {
		if eq := test.x.Equals(test.y); eq != test.exp {
			t.Errorf("expected %v.Equals(%v) == %v, got %v", test.x, test.y, test.exp, eq)
		} else if bc := test.x.Big().Cmp(test.y.Big()); (bc == 0) != eq {
			t.Errorf("Currency.Equals (%v) does not match big.Int.Cmp (%v) for %v.Equals(%v)", eq, bc, test.x, test.y)
		}
	}
}

// TestCurrencyEquals64 tests the Equals64 method for the currency type
func TestCurrencyEquals64(t *testing.T) {
	tests := []struct {
		x   Currency
		y   uint64
		exp bool
	}{
		{NewCurrency64(0), 0, true},
		{NewCurrency64(0), 1, false},
		{NewCurrency64(1), 0, false},
		{NewCurrency64(100), 7, false},
		{NewCurrency64(777), 777, true},
		{NewCurrency(big.NewInt(7)), 8, false},
	}

	for _, test := range tests {
		if eq := test.x.Equals64(test.y); eq != test.exp {
			t.Errorf("expected %v.Equals64(%v) == %v, got %v", test.x, test.y, test.exp, eq)
		} else if bc := test.x.Big().Cmp(big.NewInt(int64(test.y))); (bc == 0) != eq {
			t.Errorf("Currency.Equals64 (%v) does not match big.Int.Cmp (%v) for %v.Equals64(%v)", eq, bc, test.x, test.y)
		}
	}
}

// TestCurrencyMul probes the Mul function of the currency type.
func TestCurrencyMul(t *testing.T) {
	c5 := NewCurrency64(5)
	c6 := NewCurrency64(6)
	c30 := NewCurrency64(30)
	if c5.Mul(c6).Cmp(c30) != 0 {
		t.Error("Multiplying 5 by 6 should equal 30")
	}
}

// TestCurrencyMul64 probes the Mul64 function of the currency type.
func TestCurrencyMul64(t *testing.T) {
	c5 := NewCurrency64(5)
	u6 := uint64(6)
	c30 := NewCurrency64(30)
	if c5.Mul64(u6).Cmp(c30) != 0 {
		t.Error("Multiplying 5 by 6 should equal 30")
	}
}

// TestCurrencyMulRat probes the MulRat function of the currency type.
func TestCurrencyMulRat(t *testing.T) {
	c5 := NewCurrency64(5)
	c7 := NewCurrency64(7)
	c10 := NewCurrency64(10)
	if c5.MulRat(big.NewRat(2, 1)).Cmp(c10) != 0 {
		t.Error("Multiplying 5 by 2 should return 10")
	}
	if c5.MulRat(big.NewRat(3, 2)).Cmp(c7) != 0 {
		t.Error("Multiplying 5 by 1.5 should return 7")
	}
}

// TestCurrencyRoundDown probes the RoundDown function of the currency type.
func TestCurrencyRoundDown(t *testing.T) {
	// 10,000 is chosen because that's how many siafunds there usually are.
	c40000 := NewCurrency64(40000)
	c45000 := NewCurrency64(45000)
	if c45000.RoundDown(NewCurrency64(10000)).Cmp(c40000) != 0 {
		t.Error("rounding down 45000 to the nearest 10000 didn't work")
	}
}

// TestCurrencyIsZero probes the IsZero function of the currency type.
func TestCurrencyIsZero(t *testing.T) {
	c0 := NewCurrency64(0)
	c1 := NewCurrency64(1)
	if !c0.IsZero() {
		t.Error("IsZero returns wrong value for 0")
	}
	if c1.IsZero() {
		t.Error("IsZero returns wrong value for 1")
	}
}

// TestCurrencySqrt probes the Sqrt function of the currency type.
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

// TestCurrencySub probes the Sub function of the currency type.
func TestCurrencySub(t *testing.T) {
	c3 := NewCurrency64(3)
	c13 := NewCurrency64(13)
	c16 := NewCurrency64(16)
	if c16.Sub(c3).Cmp(c13) != 0 {
		t.Error("16 minus 3 should equal 13")
	}
}

// TestCurrencyMarshalJSON probes the MarshalJSON and UnmarshalJSON functions
// of the currency type.
func TestCurrencyMarshalJSON(t *testing.T) {
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
	if !bytes.Equal(bMar30, bytes.Trim(cMar30, `"`)) {
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

	cMar30[0] = 0
	err = cUmar30.UnmarshalJSON(cMar30)
	if err == nil {
		t.Error("JSON decoded nonsense input")
	}
}

// TestCurrencyMarshalSia probes the MarshalSia and UnmarshalSia functions of
// the currency type.
func TestCurrencyMarshalSia(t *testing.T) {
	c := NewCurrency64(1656)
	buf := new(bytes.Buffer)
	err := c.MarshalSia(buf)
	if err != nil {
		t.Fatal(err)
	}
	var cUmar Currency
	cUmar.UnmarshalSia(buf)
	if c.Cmp(cUmar) != 0 {
		t.Error("marshal and unmarshal mismatch for currency type")
	}
}

// TestCurrencyString probes the String function of the currency type.
func TestCurrencyString(t *testing.T) {
	b := big.NewInt(7135)
	c := NewCurrency64(7135)
	if b.String() != c.String() {
		t.Error("string function not behaving as expected")
	}
}

// TestCurrencyScan probes the Scan function of the currency type.
func TestCurrencyScan(t *testing.T) {
	var c0 Currency
	c1 := NewCurrency64(81293)
	_, err := fmt.Sscan("81293", &c0)
	if err != nil {
		t.Fatal(err)
	}
	if c0.Cmp(c1) != 0 {
		t.Error("scanned number does not equal expected value")
	}
	_, err = fmt.Sscan("z", &c0)
	if err == nil {
		t.Fatal("scan is accepting garbage input")
	}
}

// TestCurrencyEncoding checks that a currency can encode and decode without
// error.
func TestCurrencyEncoding(t *testing.T) {
	c := NewCurrency64(351)
	cMar := encoding.Marshal(c)
	var cUmar Currency
	err := encoding.Unmarshal(cMar, &cUmar)
	if err != nil {
		t.Error("Error unmarshalling a currency:", err)
	}
	if cUmar.Cmp(c) != 0 {
		t.Error("Marshalling and Unmarshalling a currency did not work correctly")
	}
}

// TestNegativeCurrencyMulRat checks that negative numbers are rejected when
// calling MulRat on the currency type.
func TestNegativeCurrencyMulRat(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occurred when trying to create a negative currency")
		}
	}()

	c := NewCurrency64(12)
	_ = c.MulRat(big.NewRat(-1, 1))
}

// TestNegativeCurrencySub checks that negative numbers are prevented when
// using subtraction on the currency type.
func TestNegativeCurrencySub(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occurred when trying to create a negative currency")
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
	if cNeg.i.Sign() < 0 {
		t.Error("negative currency returned")
	}
}

// TestNegativeCurrencyScan tries to scan in a negative number and checks for
// an error.
func TestNegativeCurrencyScan(t *testing.T) {
	var c Currency
	_, err := fmt.Sscan("-23", &c)
	if err != ErrNegativeCurrency {
		t.Error("expecting ErrNegativeCurrency:", err)
	}
}

// TestNegativeCurrencies tries an array of ways to produce a negative currency.
func TestNegativeNewCurrency(t *testing.T) {
	// In debug mode, attempting to get a negative currency results in a panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occurred when trying to create a negative currency")
		}
	}()

	// Try to create a new currency from a negative number.
	negBig := big.NewInt(-1)
	_ = NewCurrency(negBig)
}

// TestCurrencyUint64 tests that a currency is correctly converted to a uint64.
func TestCurrencyUint64(t *testing.T) {
	// Try a set of valid values.
	values := []uint64{0, 1, 2, 3, 4, 25e3, math.MaxUint64 - 1e6, math.MaxUint64}
	for _, value := range values {
		c := NewCurrency64(value)
		result, err := c.Uint64()
		if err != nil {
			t.Error(err)
		}
		if value != result {
			t.Error("uint64 conversion failed")
		}
	}

	// Try an overflow.
	c := NewCurrency64(math.MaxUint64)
	c = c.Mul(NewCurrency64(2))
	result, err := c.Uint64()
	if err != ErrUint64Overflow {
		t.Error(err)
	}
	if result != 0 {
		t.Error("result is not being zeroed in the event of an error")
	}
}

// TestCurrencyUnsafeDecode tests that decoding into an existing Currency
// value does not overwrite its contents.
func TestCurrencyUnsafeDecode(t *testing.T) {
	// Scan
	backup := SiacoinPrecision.Mul64(1)
	c := SiacoinPrecision
	_, err := fmt.Sscan("7", &c)
	if err != nil {
		t.Error(err)
	} else if !SiacoinPrecision.Equals(backup) {
		t.Errorf("Scan changed value of SiacoinPrecision: %v -> %v", backup, SiacoinPrecision)
	}

	// UnmarshalSia
	c = SiacoinPrecision
	err = encoding.Unmarshal(encoding.Marshal(NewCurrency64(7)), &c)
	if err != nil {
		t.Error(err)
	} else if !SiacoinPrecision.Equals(backup) {
		t.Errorf("UnmarshalSia changed value of SiacoinPrecision: %v -> %v", backup, SiacoinPrecision)
	}
}
