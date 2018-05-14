package wallet

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"
)

// TestTransactionReorg makes sure that a processedTransaction isn't returned
// by the API after bein reverted.
func TestTransactionReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create two miners
	miner1, err := siatest.NewNode(siatest.Miner(filepath.Join(testdir, "miner1")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := miner1.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	// miner1 sends a txn to itself and mines it.
	uc, err := miner1.WalletAddressGet()
	if err != nil {
		t.Fatal(err)
	}
	wsp, err := miner1.WalletSiacoinsPost(types.SiacoinPrecision, uc.Address)
	if err != nil {
		t.Fatal(err)
	}
	blocks := 1
	for i := 0; i < blocks; i++ {
		if err := miner1.MineBlock(); err != nil {
			t.Fatal(err)
		}
	}
	// wait until the transaction from before shows up as processed.
	txn := wsp.TransactionIDs[len(wsp.TransactionIDs)-1]
	err = build.Retry(100, 100*time.Millisecond, func() error {
		cg, err := miner1.ConsensusGet()
		if err != nil {
			return err
		}
		wtg, err := miner1.WalletTransactionsGet(1, cg.Height)
		if err != nil {
			return err
		}
		for _, t := range wtg.ConfirmedTransactions {
			if t.TransactionID == txn {
				return nil
			}
		}
		return errors.New("txn isn't processed yet")
	})
	if err != nil {
		t.Fatal(err)
	}
	miner2, err := siatest.NewNode(siatest.Miner(filepath.Join(testdir, "miner2")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := miner2.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// miner2 mines 2 blocks now to create a longer chain than miner1.
	for i := 0; i < blocks+1; i++ {
		if err := miner2.MineBlock(); err != nil {
			t.Fatal(err)
		}
	}
	// miner1 and miner2 connect. This should cause a reorg that reverts the
	// transaction from before.
	if err := miner1.GatewayConnectPost(miner2.GatewayAddress()); err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, 100*time.Millisecond, func() error {
		cg, err := miner1.ConsensusGet()
		if err != nil {
			return err
		}
		wtg, err := miner1.WalletTransactionsGet(1, cg.Height)
		if err != nil {
			return err
		}
		for _, t := range wtg.ConfirmedTransactions {
			if t.TransactionID == txn {
				return errors.New("txn is still processed")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

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
	for _, output := range unspentResp.Outputs {
		if output.ID == types.OutputID(txn.SiacoinInputs[0].ParentID) {
			t.Fatal("spent output still listed as spendable")
		}
	}
}
