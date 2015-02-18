package consensus

import (
	"testing"
)

// testApplySiacoinOutput gets a transaction with a siacoin output and puts the
// transaction into the blockchain, then checks that the output made it into
// the consensus set.
func (a *Assistant) testApplySiacoinOutput() {
	// Grab a transcation with a siacoin output and put it into the blockchain.
	txn := a.SiacoinOutputTransaction()
	block, err := a.MineCurrentBlock([]Transaction{txn})
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Check that the output got added to the consensus set.
	_, exists := a.State.siacoinOutputs[txn.SiacoinOutputID(0)]
	if !exists {
		a.Tester.Fatal("siacoin output did not make it into the unspent outputs set.")
	}
}

// TestApplySiacoinOutput creates a new testing environment and uses it to call
// testApplySiacoinOutputs.
func TestApplySiacoinOutput(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testApplySiacoinOutput()
}
