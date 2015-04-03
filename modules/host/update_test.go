package host

import (
	"crypto/rand"
	// "testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// testObligation adds a file obligation to the host's set of obligations, then
// mines blocks and updates the host, causing the host to submit a storage
// proof. Then the storage proof is mined and a check is made to see that the
// host gets the payout.
func (ht *HostTester) testObligation() {
	// Allocate the file that the host is required to store.
	filesize := uint64(4e3)
	data := make([]byte, filesize)
	rand.Read(data)
	file, path, err := ht.allocate(filesize)
	if err != nil {
		ht.Fatal(err)
	}
	_, err = file.Write(data)
	if err != nil {
		ht.Fatal(err)
	}
	file.Seek(0, 0)

	// Create, finance, and mine a transaction with a file contract in it using
	// the data's merkle root.
	input, value := ht.FindSpendableSiacoinInput()
	txn := ht.AddSiacoinInputToTransaction(types.Transaction{}, input)
	merkleRoot, err := crypto.ReaderMerkleRoot(file)
	if err != nil {
		ht.Fatal(err)
	}
	file.Close()
	fc := types.FileContract{
		FileSize:       filesize,
		FileMerkleRoot: merkleRoot,
		Start:          ht.State.Height() + 2,
		Expiration:     ht.State.Height() + 3,
		Payout:         value,
		ValidProofOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{Value: value, UnlockHash: ht.Host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{Value: value, UnlockHash: types.ZeroUnlockHash},
		},
	}
	fc.ValidProofOutputs[0].Value = fc.ValidProofOutputs[0].Value.Sub(fc.Tax())
	txn.FileContracts = append(txn.FileContracts, fc)
	ht.MineAndSubmitCurrentBlock([]types.Transaction{txn})

	// Add the obligation for the file to the host.
	fcid := txn.FileContractID(0)
	co := contractObligation{
		ID:           fcid,
		FileContract: fc,
		Path:         path,
	}
	ht.mu.Lock()
	ht.obligationsByHeight[ht.Height()+1] = append(ht.obligationsByHeight[ht.Height()+1], co)
	ht.obligationsByID[fcid] = co
	ht.mu.Unlock()

	// Get the balance before the proof is submitted, mine enough blocks to
	// have the proof submitted, then check that the balance experienced an
	// increase.
	startingBalance := ht.wallet.Balance(true)
	for i := types.BlockHeight(0); i < 3+types.MaturityDelay; i++ {
		ht.Host.update()
		tSet := ht.Host.tpool.TransactionSet()
		ht.MineAndSubmitCurrentBlock(tSet)
	}
	if startingBalance.Cmp(ht.wallet.Balance(true)) >= 0 {
		// TODO: Fix this test.
		//
		// ht.Error("balance did not increase after submitting and maturing a storage proof")
	}
}

/*
// TestObligation creates a host tester and calls testObligation.
func TestObligation(t *testing.T) {
	ht := CreateHostTester("TestObligation", t)
	ht.testObligation()
}
*/
