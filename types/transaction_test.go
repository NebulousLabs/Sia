package types

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// TestTransactionIDs probes all of the ID functions of the transaction type.
func TestIDs(t *testing.T) {
	// Create every type of ID using empty fields.
	txn := Transaction{
		SiacoinOutputs: []SiacoinOutput{
			SiacoinOutput{},
		},
		FileContracts: []FileContract{
			FileContract{},
		},
		SiafundOutputs: []SiafundOutput{
			SiafundOutput{},
		},
	}
	tid := txn.ID()
	scoid := txn.SiacoinOutputID(0)
	fcid := txn.FileContractID(0)
	fctpid := fcid.FileContractTerminationPayoutID(0)
	spidT := fcid.StorageProofOutputID(true, 0)
	spidF := fcid.StorageProofOutputID(false, 0)
	sfoid := txn.SiafundOutputID(0)
	scloid := sfoid.SiaClaimOutputID()

	// Put all of the ids into a slice.
	var ids []crypto.Hash
	ids = append(ids,
		crypto.Hash(tid),
		crypto.Hash(scoid),
		crypto.Hash(fcid),
		crypto.Hash(fctpid),
		crypto.Hash(spidT),
		crypto.Hash(spidF),
		crypto.Hash(sfoid),
		crypto.Hash(scloid),
	)

	// Check that each id is unique.
	knownIDs := make(map[crypto.Hash]struct{})
	for i, id := range ids {
		_, exists := knownIDs[id]
		if exists {
			t.Error("id repeat for index", i)
		}
		knownIDs[id] = struct{}{}
	}
}

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
