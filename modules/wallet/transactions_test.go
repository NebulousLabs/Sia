package wallet

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationTransactions checks that the transaction history is being
// correctly recorded and extended.
func TestIntegrationTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Creating the wallet tester results in blocks being mined until the miner
	// has money, which means types.MaturityDelay+1 blocks are created, and
	// each block is going to have a transaction (the miner payout) going to
	// the wallet.
	txns, err := wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	sentValue := types.NewCurrency64(5000)
	_, err = wt.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	// No more confirmed transactions have been added.
	txns, err = wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	// Two transactions added to unconfirmed pool - 1 to fund the exact output,
	// and 1 to hold the exact output.
	utxns, err := wt.wallet.UnconfirmedTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(utxns) != 2 {
		t.Error("was expecting 4 unconfirmed transactions")
	}

	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	// A confirmed transaction was added for the miner payout, and the 2
	// transactions that were previously unconfirmed.
	txns, err = wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+2+2) {
		t.Errorf("unexpected transaction history length: expected %v, got %v", types.MaturityDelay+2+2, len(txns))
	}

	// Try getting a partial history for just the previous block.
	txns, err = wt.wallet.Transactions(types.MaturityDelay+2, types.MaturityDelay+2)
	if err != nil {
		t.Fatal(err)
	}
	// The partial should include one transaction for a block, and 2 for the
	// send that occurred.
	if len(txns) != 3 {
		t.Errorf("unexpected transaction history length: expected %v, got %v", 3, len(txns))
	}
}

// TestTransactionsSingleTxn checks if it is possible to find a txn that was
// appended to the processed transactions and is also the only txn for a
// certain block height.
func TestTransactionsSingleTxn(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Creating the wallet tester results in blocks being mined until the miner
	// has money, which means types.MaturityDelay+1 blocks are created, and
	// each block is going to have a transaction (the miner payout) going to
	// the wallet.
	txns, err := wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}

	// Create a processed txn for a future block height. It whould be the last
	// txn in the database and the only txn with that height.
	height := wt.cs.Height() + 1
	pt := modules.ProcessedTransaction{
		ConfirmationHeight: height,
	}
	wt.wallet.mu.Lock()
	if err := dbAppendProcessedTransaction(wt.wallet.dbTx, pt); err != nil {
		t.Fatal(err)
	}

	// Set the consensus height to height. Otherwise Transactions will return
	// an error. We can't just mine a block since that would create new
	// transactions.
	if err := dbPutConsensusHeight(wt.wallet.dbTx, height); err != nil {
		t.Fatal(err)
	}
	wt.wallet.mu.Unlock()

	// Search for the previously appended txn
	txns, err = wt.wallet.Transactions(height, height)
	if err != nil {
		t.Fatal(err)
	}

	// We should find exactly 1 txn
	if len(txns) != 1 {
		t.Errorf("Found %v txns but should be 1", len(txns))
	}
}

// TestIntegrationTransaction checks that individually queried transactions
// contain the correct values.
func TestIntegrationTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	_, exists, err := wt.wallet.Transaction(types.TransactionID{})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("able to query a nonexisting transction")
	}

	// test sending siacoins
	sentValue := types.NewCurrency64(5000)
	sendTxns, err := wt.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// sendTxns[0] is the set-up transaction, sendTxns[1] contains the sentValue output
	txn, exists, err := wt.wallet.Transaction(sendTxns[1].ID())
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("unable to query transaction")
	}
	if txn.TransactionID != sendTxns[1].ID() {
		t.Error("wrong transaction was fetched")
	} else if len(txn.Inputs) != 1 || len(txn.Outputs) != 2 {
		t.Error("expected 1 input and 2 outputs, got", len(txn.Inputs), len(txn.Outputs))
	} else if !txn.Outputs[0].Value.Equals(sentValue) {
		t.Errorf("expected first output to equal %v, got %v", sentValue, txn.Outputs[0].Value)
	} else if exp := txn.Inputs[0].Value.Sub(sentValue); !txn.Outputs[1].Value.Equals(exp) {
		t.Errorf("expected first output to equal %v, got %v", exp, txn.Outputs[1].Value)
	}

	// test sending siafunds
	err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of1of1.siakey"})
	if err != nil {
		t.Error(err)
	}
	sentValue = types.NewCurrency64(12)
	sendTxns, err = wt.wallet.SendSiafunds(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	txn, exists, err = wt.wallet.Transaction(sendTxns[1].ID())
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("unable to query transaction")
	}
	if len(txn.Inputs) != 1 || len(txn.Outputs) != 3 {
		t.Error("expected 1 input and 3 outputs, got", len(txn.Inputs), len(txn.Outputs))
	} else if !txn.Outputs[1].Value.Equals(sentValue) {
		t.Errorf("expected first output to equal %v, got %v", sentValue, txn.Outputs[1].Value)
	} else if exp := txn.Inputs[0].Value.Sub(sentValue); !txn.Outputs[2].Value.Equals(exp) {
		t.Errorf("expected first output to equal %v, got %v", exp, txn.Outputs[2].Value)
	}
}

// TestProcessedTxnIndexCompatCode checks if the compatibility code for the
// bucketProcessedTxnIndex works as expected
func TestProcessedTxnIndexCompatCode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Mine blocks to get lots of processed transactions
	for i := 0; i < 1000; i++ {
		if _, err := wt.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}

	// The wallet tester mined blocks. Therefore the bucket shouldn't be
	// empty.
	wt.wallet.mu.Lock()
	wt.wallet.syncDB()
	expectedTxns := wt.wallet.dbTx.Bucket(bucketProcessedTxnIndex).Stats().KeyN
	if expectedTxns == 0 {
		t.Fatal("bucketProcessedTxnIndex shouldn't be empty")
	}

	// Delete the bucket
	if err := wt.wallet.dbTx.DeleteBucket(bucketProcessedTxnIndex); err != nil {
		t.Fatalf("Failed to empty bucket: %v", err)
	}

	// Bucket shouldn't exist
	if wt.wallet.dbTx.Bucket(bucketProcessedTxnIndex) != nil {
		t.Fatal("bucketProcessedTxnIndex should be empty")
	}
	wt.wallet.mu.Unlock()

	// Close the wallet
	if err := wt.wallet.Close(); err != nil {
		t.Fatalf("Failed to close wallet: %v", err)
	}

	// Restart wallet
	wallet, err := New(wt.cs, wt.tpool, filepath.Join(wt.persistDir, modules.WalletDir))
	if err != nil {
		t.Fatalf("Failed to restart wallet: %v", err)
	}
	wt.wallet = wallet

	// Bucket should exist
	wt.wallet.mu.Lock()
	defer wt.wallet.mu.Unlock()
	if wt.wallet.dbTx.Bucket(bucketProcessedTxnIndex) == nil {
		t.Fatal("bucketProcessedTxnIndex should exist")
	}

	// Check if bucket has expected size
	wt.wallet.syncDB()
	numTxns := wt.wallet.dbTx.Bucket(bucketProcessedTxnIndex).Stats().KeyN
	if expectedTxns != numTxns {
		t.Errorf("Bucket should have %v entries but had %v", expectedTxns, numTxns)
	}
}

// TestIntegrationAddressTransactions checks grabbing the history for a single
// address.
func TestIntegrationAddressTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Grab an address and send it money.
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5005), addr)
	if err != nil {
		t.Fatal(err)
	}

	// Check the confirmed balance of the address.
	addrHist, err := wt.wallet.AddressTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrHist) != 0 {
		t.Error("address should be empty - no confirmed transactions")
	}
	utxns, err := wt.wallet.AddressUnconfirmedTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(utxns) == 0 {
		t.Error("addresses unconfirmed transactions should not be empty")
	}
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	addrHist, err = wt.wallet.AddressTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrHist) == 0 {
		t.Error("address history should have some transactions")
	}
	utxns, err = wt.wallet.AddressUnconfirmedTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(utxns) != 0 {
		t.Error("addresses unconfirmed transactions should be empty")
	}
}

// TestAddressTransactionRevertedBlock checks grabbing the history for a
// address after its block was reverted
func TestAddressTransactionRevertedBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Grab an address and send it money.
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5005), addr)
	if err != nil {
		t.Fatal(err)
	}

	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	addrHist, err := wt.wallet.AddressTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrHist) == 0 {
		t.Error("address history should have some transactions")
	}
	utxns, err := wt.wallet.AddressUnconfirmedTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(utxns) != 0 {
		t.Error("addresses unconfirmed transactions should be empty")
	}

	// Revert the block
	wt.wallet.mu.Lock()
	if err := wt.wallet.revertHistory(wt.wallet.dbTx, []types.Block{b}); err != nil {
		t.Fatal(err)
	}
	wt.wallet.mu.Unlock()

	addrHist, err = wt.wallet.AddressTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrHist) > 0 {
		t.Error("address history should should be empty")
	}
	utxns, err = wt.wallet.AddressUnconfirmedTransactions(addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(utxns) > 0 {
		t.Error("addresses unconfirmed transactions should have some transactions")
	}
}

// TestTransactionInputOutputIDs verifies that ProcessedTransaction's inputs
// and outputs have a valid ID field.
func TestTransactionInputOutputIDs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// mine a few blocks to create miner payouts
	for i := 0; i < 5; i++ {
		_, err = wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// create some siacoin outputs
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5005), addr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// verify the miner payouts and siacoin outputs/inputs have correct IDs
	txns, err := wt.wallet.Transactions(0, 1000)
	if err != nil {
		t.Fatal(err)
	}

	outputIDs := make(map[types.OutputID]struct{})
	for _, txn := range txns {
		block, _ := wt.cs.BlockAtHeight(txn.ConfirmationHeight)
		for i, output := range txn.Outputs {
			outputIDs[output.ID] = struct{}{}
			if output.FundType == types.SpecifierMinerPayout {
				if output.ID != types.OutputID(block.MinerPayoutID(uint64(i))) {
					t.Fatal("miner payout had incorrect output ID")
				}
			}
			if output.FundType == types.SpecifierSiacoinOutput {
				if output.ID != types.OutputID(txn.Transaction.SiacoinOutputID(uint64(i))) {
					t.Fatal("siacoin output had incorrect output ID")
				}
			}
		}
		for _, input := range txn.Inputs {
			if _, exists := outputIDs[input.ParentID]; !exists {
				t.Fatal("input has ParentID that points to a nonexistent output:", input.ParentID)
			}
		}
	}
}

// BenchmarkAddressTransactions benchmarks the AddressTransactions method,
// using the near-worst-case scenario of 10,000 transactions to search through
// with only a single relevant transaction.
func BenchmarkAddressTransactions(b *testing.B) {
	wt, err := createWalletTester(b.Name(), modules.ProdDependencies)
	if err != nil {
		b.Fatal(err)
	}
	// add a bunch of fake transactions to the db
	//
	// NOTE: this is somewhat brittle, but the alternative (generating
	// authentic transactions) is prohibitively slow.
	wt.wallet.mu.Lock()
	for i := 0; i < 10000; i++ {
		err := dbAppendProcessedTransaction(wt.wallet.dbTx, modules.ProcessedTransaction{
			TransactionID: types.TransactionID{1},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	// add a single relevant transaction
	searchAddr := types.UnlockHash{1}
	err = dbAppendProcessedTransaction(wt.wallet.dbTx, modules.ProcessedTransaction{
		TransactionID: types.TransactionID{1},
		Inputs: []modules.ProcessedInput{{
			RelatedAddress: searchAddr,
		}},
	})
	if err != nil {
		b.Fatal(err)
	}
	wt.wallet.syncDB()
	wt.wallet.mu.Unlock()

	b.ResetTimer()
	b.Run("indexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			txns, err := wt.wallet.AddressTransactions(searchAddr)
			if err != nil {
				b.Fatal(err)
			}
			if len(txns) != 1 {
				b.Fatal(len(txns))
			}
		}
	})
	b.Run("indexed-nosync", func(b *testing.B) {
		wt.wallet.db.NoSync = true
		for i := 0; i < b.N; i++ {
			txns, err := wt.wallet.AddressTransactions(searchAddr)
			if err != nil {
				b.Fatal(err)
			}
			if len(txns) != 1 {
				b.Fatal(len(txns))
			}
		}
		wt.wallet.db.NoSync = false
	})
	b.Run("unindexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			wt.wallet.mu.Lock()
			wt.wallet.syncDB()
			var pts []modules.ProcessedTransaction
			it := dbProcessedTransactionsIterator(wt.wallet.dbTx)
			for it.next() {
				pt := it.value()
				relevant := false
				for _, input := range pt.Inputs {
					relevant = relevant || input.RelatedAddress == searchAddr
				}
				for _, output := range pt.Outputs {
					relevant = relevant || output.RelatedAddress == searchAddr
				}
				if relevant {
					pts = append(pts, pt)
				}
			}
			_ = pts
			wt.wallet.mu.Unlock()
		}
	})
}
