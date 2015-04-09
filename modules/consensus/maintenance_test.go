package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// testApplyMissedProof creates a contract and puts it into the blockchain, and
// then checks that the payouts were added.
func (ct *ConsensusTester) testApplyMissedProof() {
	txn, _ := ct.FileContractTransaction(ct.Height()+2, ct.Height()+3)
	block := ct.MineCurrentBlock([]types.Transaction{txn})
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		block = ct.MineCurrentBlock(nil)
		err = ct.AcceptBlock(block)
		if err != nil {
			ct.Fatal(err)
		}
	}

	// Check that the contract has been deleted, and that the missed proof
	// outputs have been created.
	fcid := txn.FileContractID(0)
	_, exists := ct.fileContracts[fcid]
	if exists {
		ct.Error("file contract not removed from consensus set upon missed storage proof")
	}
	outputs, exists := ct.delayedSiacoinOutputs[ct.Height()]
	if !exists {
		ct.Fatal("missed proof outputs not created")
	}
	_, exists = outputs[fcid.StorageProofOutputID(false, 0)]
	if !exists {
		ct.Error("missed proof outputs not created")
	}

}

// TestApplyMissedProof creates a new testing environment and uses it to call
// testApplyMissedProof.
func TestApplyMissedProof(t *testing.T) {
	ct := NewTestingEnvironment("TestApplyMissedProof", t)
	ct.testApplyMissedProof()
}
