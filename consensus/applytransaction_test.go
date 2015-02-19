package consensus

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
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
		a.Tester.Fatal("siacoin output did not make it into the consensus set.")
	}
}

// testApplyFileContract gets a transaction with file contract creation and
// puts it into the blockchain, then checks that the file contract has appeared
// in the consensus set.
func (a *Assistant) testApplyFileContract() {
	// Grab a transction with a file contract and put it into the blockchain.
	txn, _ := a.FileContractTransaction(a.State.Height()+2, a.State.Height()+3)
	block, err := a.MineCurrentBlock([]Transaction{txn})
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Check for the file contract in the consensus set.
	_, exists := a.State.fileContracts[txn.FileContractID(0)]
	if !exists {
		a.Tester.Fatal("file contract did not make it into the consensus set.")
	}
}

// testApplyStorageProof gets a transaction with file contract creation and
// puts it into the blockchain, then submits a storage proof for the file and
// checks that the payout was properly distributed.
func (a *Assistant) testApplyStorageProof() {
	// Grab a transction with a file contract and put it into the blockchain.
	fcTxn, file := a.FileContractTransaction(a.State.Height()+2, a.State.Height()+3)
	fcid := fcTxn.FileContractID(0)
	block, err := a.MineCurrentBlock([]Transaction{fcTxn})
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Mine blocks until the file contract is active.
	for a.State.Height() < fcTxn.FileContracts[0].Start {
		block, err := a.MineCurrentBlock(nil)
		if err != nil {
			a.Tester.Fatal(err)
		}
		err = a.State.AcceptBlock(block)
		if err != nil {
			a.Tester.Fatal(err)
		}
	}

	// Create the storage proof.
	segmentIndex, err := a.State.StorageProofSegment(fcid)
	if err != nil {
		a.Tester.Fatal(err)
	}
	reader := bytes.NewReader(file)
	base, hashSet, err := crypto.BuildReaderProof(reader, crypto.CalculateSegments(uint64(len(file))), segmentIndex)
	if err != nil {
		a.Tester.Fatal(err)
	}
	sp := StorageProof{fcid, base, hashSet}

	// Put the storage proof in the blockchain.
	proofTxn := Transaction{}
	proofTxn.StorageProofs = append(proofTxn.StorageProofs, sp)
	block, err = a.MineCurrentBlock([]Transaction{proofTxn})
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Check that the file contract was deleted from the consensus set, and
	// that the delayed outputs for the successful proof were added.
	_, exists := a.State.fileContracts[fcid]
	if exists {
		a.Tester.Error("file contract not deleted from consensus set")
	}
	delayedOutputs, exists := a.State.delayedSiacoinOutputs[a.State.Height()]
	if !exists {
		a.Tester.Fatal("delayed outputs don't seem to exist")
	}
	_, exists = delayedOutputs[fcid.StorageProofOutputID(true, 0)]
	if !exists {
		a.Tester.Fatal("delayed outputs don't seem to exist, but height map does")
	}
}

// TestApplySiacoinOutput creates a new testing environment and uses it to call
// testApplySiacoinOutput.
func TestApplySiacoinOutput(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testApplySiacoinOutput()
}

// TestApplyFileContract creates a new testing environment and uses it to call
// testApplyFileContract.
func TestApplyFileContract(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testApplyFileContract()
}

// TestApplyStorageProof creates a new testing environment and uses it to call
// testApplyStorageProof.
func TestApplyStorageProof(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	a := NewTestingEnvironment(t)
	a.testApplyStorageProof()
}
