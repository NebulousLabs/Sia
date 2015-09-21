package types

import (
	"testing"
)

// TestFileContractTax probes the Tax function.
func TestTax(t *testing.T) {
	// Test explicit values for post-hardfork tax values.
	if Tax(1e9, NewCurrency64(125e9)).Cmp(NewCurrency64(4875e6)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}
	if PostTax(1e9, NewCurrency64(125e9)).Cmp(NewCurrency64(120125e6)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}

	// Test equivalency for a series of values.
	if testing.Short() {
		t.SkipNow()
	}
	// COMPATv0.4.0 - check at height 0.
	for i := uint64(0); i < 10e3; i++ {
		val := NewCurrency64((1e3 * i) + i)
		tax := Tax(0, val)
		postTax := PostTax(0, val)
		if val.Cmp(tax.Add(postTax)) != 0 {
			t.Error("tax calculation inconsistent for", i)
		}
	}
	// Check at height 1e9
	for i := uint64(0); i < 10e3; i++ {
		val := NewCurrency64((1e3 * i) + i)
		tax := Tax(1e9, val)
		postTax := PostTax(1e9, val)
		if val.Cmp(tax.Add(postTax)) != 0 {
			t.Error("tax calculation inconsistent for", i)
		}
	}
}
