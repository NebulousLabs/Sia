package types

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/crypto"
)

// TestTransactionIDs probes all of the ID functions of the Transaction type.
func TestIDs(t *testing.T) {
	// Create every type of ID using empty fields.
	txn := Transaction{
		SiacoinOutputs: []SiacoinOutput{{}},
		FileContracts:  []FileContract{{}},
		SiafundOutputs: []SiafundOutput{{}},
	}
	tid := txn.ID()
	scoid := txn.SiacoinOutputID(0)
	fcid := txn.FileContractID(0)
	spidT := fcid.StorageProofOutputID(ProofValid, 0)
	spidF := fcid.StorageProofOutputID(ProofMissed, 0)
	sfoid := txn.SiafundOutputID(0)
	scloid := sfoid.SiaClaimOutputID()

	// Put all of the ids into a slice.
	var ids []crypto.Hash
	ids = append(ids,
		crypto.Hash(tid),
		crypto.Hash(scoid),
		crypto.Hash(fcid),
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

// TestTransactionSiacoinOutputSum probes the SiacoinOutputSum method of the
// Transaction type.
func TestTransactionSiacoinOutputSum(t *testing.T) {
	// Create a transaction with all types of siacoin outputs.
	txn := Transaction{
		SiacoinOutputs: []SiacoinOutput{
			{Value: NewCurrency64(1)},
			{Value: NewCurrency64(20)},
		},
		FileContracts: []FileContract{
			{Payout: NewCurrency64(300)},
			{Payout: NewCurrency64(4000)},
		},
		MinerFees: []Currency{
			NewCurrency64(50000),
			NewCurrency64(600000),
		},
	}
	if txn.SiacoinOutputSum().Cmp(NewCurrency64(654321)) != 0 {
		t.Error("wrong siacoin output sum was calculated, got:", txn.SiacoinOutputSum())
	}
}
