package types

import (
	"testing"
)

// TestFileContractTax probes the Tax function.
func TestTax(t *testing.T) {
	fc := FileContract{
		Payout: NewCurrency64(435000),
	}
	if fc.Tax().Cmp(NewCurrency64(10000)) != 0 {
		t.Error("Tax producing unexpected result")
	}
	fc.Payout = NewCurrency64(150000)
	if fc.Tax().Cmp(NewCurrency64(0)) != 0 {
		t.Error("Tax producing unexpected result")
	}
	fc.Payout = NewCurrency64(123456789)
	if fc.Tax().Cmp(NewCurrency64(4810000)) != 0 {
		t.Error("Tax producing unexpected result")
	}
}
