package modules

import (
	"testing"
)

// TestMaxFileContractSetLenSanity checks that a sensible value for
// MaxFileContractSetLen has been chosen.
func TestMaxFileContractSetLenSanity(t *testing.T) {
	// It does not make sense for the contract set limit to be higher than the
	// IsStandard limit in the transaction pool. Such a transaction set would
	// never be accepted by the transaction pool, and therefore is going to
	// cause a failure later on in the host process. An extra 1kb is left
	// because the file contract transaction is going to grow as the terms are
	// negotiated and as signatures are added.
	if MaxFileContractSetLen > TransactionSetSizeLimit - 1e3 {
		t.Fatal("MaxfileContractSetLen does not have a sensible value - should be smaller than the TransactionSetSizeLimit")
	}

}
