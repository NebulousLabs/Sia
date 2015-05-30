package consensus

/*
import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// testApplyStorageProof gets a transaction with file contract creation and
// puts it into the blockchain, then submits a storage proof for the file and
// checks that the payout was properly distributed.
func (ct *ConsensusTester) testApplyStorageProof() {
	// Grab a transction with a file contract and put it into the blockchain.
	fcTxn, file := ct.FileContractTransaction(ct.Height()+2, ct.Height()+3)
	fcid := fcTxn.FileContractID(0)
	block := ct.MineCurrentBlock([]types.Transaction{fcTxn})
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}

	// Mine blocks until the file contract is active.
	for ct.Height() < fcTxn.FileContracts[0].WindowStart {
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
	sp := types.StorageProof{fcid, base, hashSet}

	// Put the storage proof in the blockchain.
	proofTxn := types.Transaction{}
	proofTxn.StorageProofs = append(proofTxn.StorageProofs, sp)
	block = ct.MineCurrentBlock([]types.Transaction{proofTxn})
	err = ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err) // TODO: Occasionally fails, not sure why.
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

// TestApplyStorageProof creates a new testing environment and uses it to call
// testApplyStorageProof.
func TestApplyStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ct := NewTestingEnvironment("TestApplyStorageProof", t)
	ct.testApplyStorageProof()
}
*/
