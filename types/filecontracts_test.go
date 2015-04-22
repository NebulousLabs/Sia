package types

import (
	"testing"
)

// TestFileContractTax probes the Tax function.
func TestTax(t *testing.T) {
	if SiafundPortion != 0.039 {
		t.Error("SiafundPortion does not match expected value, Tax testing may be off")
	}
	if SiafundCount != 10000 {
		t.Error("SiafundCount does not match expected value, Tax testing may be off")
	}

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
