package consensus

import (
	"testing"
)

// testApplyMissedProof creates a contract and puts it into the blockchain, and
// then checks that the payouts were added.
func (a *Assistant) testApplyMissedProof() {
	txn, _ := a.FileContractTransaction(a.State.Height()+2, a.State.Height()+3)
	block, err := a.MineCurrentBlock([]Transaction{txn})
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		block, err = a.MineCurrentBlock(nil)
		if err != nil {
			a.Tester.Fatal(err)
		}
		err = a.State.AcceptBlock(block)
		if err != nil {
			a.Tester.Fatal(err)
		}
	}

	// Check that the contract has been deleted, and that the missed proof
	// outputs have been created.
	fcid := txn.FileContractID(0)
	_, exists := a.State.fileContracts[fcid]
	if exists {
		a.Tester.Error("file contract not removed from consensus set upon missed storage proof")
	}
	outputs, exists := a.State.delayedSiacoinOutputs[a.State.Height()]
	if !exists {
		a.Tester.Fatal("missed proof outputs not created")
	}
	_, exists = outputs[fcid.StorageProofOutputID(false, 0)]
	if !exists {
		a.Tester.Error("missed proof outputs not created")
	}

}

// TestApplyMissedProof creates a new testing environment and uses it to call
// testApplyMissedProof.
func TestApplyMissedProof(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testApplyMissedProof()
}
