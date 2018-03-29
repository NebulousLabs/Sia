package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"
)

// TestSignTransaction is a integration test for signing transaction offline
// using the API.
func TestSignTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a new server
	testNode, err := siatest.NewNode(node.AllModules(testdir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// get an output to spend
	unspentResp, err := testNode.WalletUnspentGet()
	if err != nil {
		t.Fatal("failed to get spendable outputs")
	}
	outputs := unspentResp.Outputs

	// create a transaction that sends an output to the void
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: types.SiacoinOutputID(outputs[0].ID),
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      outputs[0].Value,
			UnlockHash: types.UnlockHash{},
		}},
	}

	// sign the transaction
	signResp, err := testNode.WalletSignPost(txn, map[types.OutputID]types.UnlockHash{
		outputs[0].ID: outputs[0].UnlockHash,
	})
	if err != nil {
		t.Fatal("failed to sign the transaction", err)
	}
	txn = signResp.Transaction

	// txn should now have unlock condictions and a signature
	if txn.SiacoinInputs[0].UnlockConditions.SignaturesRequired == 0 {
		t.Fatal("unlock conditions are still unset")
	}
	if len(txn.TransactionSignatures) == 0 {
		t.Fatal("transaction was not signed")
	}

	// the resulting transaction should be valid; submit it to the tpool and
	// mine a block to confirm it
	if err := testNode.TransactionpoolRawPost(nil, txn); err != nil {
		t.Fatal("failed to add transaction to pool", err)
	}
	if err := testNode.MineBlock(); err != nil {
		t.Fatal("failed to mine block", err)
	}

	// the wallet should no longer list the resulting output as spendable
	unspentResp, err = testNode.WalletUnspentGet()
	if err != nil {
		t.Fatal("failed to get spendable outputs")
	}
	outputs = unspentResp.Outputs
	if len(outputs) != 1 {
		t.Fatal("expected one output")
	}
	if outputs[0].ID == types.OutputID(txn.SiacoinInputs[0].ParentID) {
		t.Fatal("spent output still listed as spendable")
	}
}
