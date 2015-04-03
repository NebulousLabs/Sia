package consensus

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// testApplySiacoinOutput gets a transaction with a siacoin output and puts the
// transaction into the blockchain, then checks that the output made it into
// the consensus set.
func (ct *ConsensusTester) testApplySiacoinOutput() {
	// Grab a transcation with a siacoin output and put it into the blockchain.
	txn := ct.SiacoinOutputTransaction()
	block := ct.MineCurrentBlock([]Transaction{txn})
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}

	// Check that the output got added to the consensus set.
	_, exists := ct.siacoinOutputs[txn.SiacoinOutputID(0)]
	if !exists {
		ct.Fatal("siacoin output did not make it into the consensus set.")
	}
}

// testApplyFileContract gets a transaction with file contract creation and
// puts it into the blockchain, then checks that the file contract has appeared
// in the consensus set.
func (ct *ConsensusTester) testApplyFileContract() {
	// Grab a transction with a file contract and put it into the blockchain.
	txn, _ := ct.FileContractTransaction(ct.Height()+2, ct.Height()+3)
	block := ct.MineCurrentBlock([]Transaction{txn})
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}

	// Check for the file contract in the consensus set.
	_, exists := ct.fileContracts[txn.FileContractID(0)]
	if !exists {
		ct.Fatal("file contract did not make it into the consensus set.")
	}
}

// testApplyStorageProof gets a transaction with file contract creation and
// puts it into the blockchain, then submits a storage proof for the file and
// checks that the payout was properly distributed.
func (ct *ConsensusTester) testApplyStorageProof() {
	// Grab a transction with a file contract and put it into the blockchain.
	fcTxn, file := ct.FileContractTransaction(ct.Height()+2, ct.Height()+3)
	fcid := fcTxn.FileContractID(0)
	block := ct.MineCurrentBlock([]Transaction{fcTxn})
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}

	// Mine blocks until the file contract is active.
	for ct.Height() < fcTxn.FileContracts[0].Start {
		block := ct.MineCurrentBlock(nil)
		err := ct.AcceptBlock(block)
		if err != nil {
			ct.Fatal(err)
		}
	}

	// Create the storage proof.
	segmentIndex, err := ct.StorageProofSegment(fcid)
	if err != nil {
		ct.Fatal(err)
	}
	reader := bytes.NewReader(file)
	base, hashSet, err := crypto.BuildReaderProof(reader, segmentIndex)
	if err != nil {
		ct.Fatal(err)
	}
	sp := StorageProof{fcid, base, hashSet}

	// Put the storage proof in the blockchain.
	proofTxn := Transaction{}
	proofTxn.StorageProofs = append(proofTxn.StorageProofs, sp)
	block = ct.MineCurrentBlock([]Transaction{proofTxn})
	err = ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}

	// Check that the file contract was deleted from the consensus set, and
	// that the delayed outputs for the successful proof were added.
	_, exists := ct.fileContracts[fcid]
	if exists {
		ct.Error("file contract not deleted from consensus set")
	}
	delayedOutputs, exists := ct.delayedSiacoinOutputs[ct.Height()]
	if !exists {
		ct.Fatal("delayed outputs don't seem to exist")
	}
	_, exists = delayedOutputs[fcid.StorageProofOutputID(true, 0)]
	if !exists {
		ct.Fatal("delayed outputs don't seem to exist, but height map does")
	}
}

// TestApplySiacoinOutput creates a new testing environment and uses it to call
// testApplySiacoinOutput.
func TestApplySiacoinOutput(t *testing.T) {
	ct := NewTestingEnvironment(t)
	ct.testApplySiacoinOutput()
}

// TestApplyFileContract creates a new testing environment and uses it to call
// testApplyFileContract.
func TestApplyFileContract(t *testing.T) {
	ct := NewTestingEnvironment(t)
	ct.testApplyFileContract()
}

// TestApplyStorageProof creates a new testing environment and uses it to call
// testApplyStorageProof.
func TestApplyStorageProof(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testApplyStorageProof()
}
