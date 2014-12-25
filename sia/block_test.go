package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testEmptyBlock creates an emtpy block and submits it to the state, checking that a utxo is created for the miner subisdy.
func testEmptyBlock(t *testing.T, e *Environment) {
	// Check that the block will actually be empty.
	if len(e.state.TransactionPoolDump()) != 0 {
		t.Error("TransactionPoolDump is not of len 0")
		return
	}

	// Create and submit the block.
	height := e.Height()
	utxoSize := len(e.state.SortedUtxoSet())
	mineSingleBlock(t, e)
	if height+1 != e.Height() {
		t.Errorf("height should have increased by one, went from %v to %v.", height, e.Height())
	}
	if utxoSize+1 != len(e.state.SortedUtxoSet()) {
		t.Errorf("utxo set should have increased by one, went from %v to %v.", utxoSize, len(e.state.SortedUtxoSet()))
	}
}

// testTransactionBlock creates a transaction and checks that it makes it into
// the utxo set.
func testTransactionBlock(t *testing.T, e *Environment) {
	// As a prereq the balance of the wallet needs to be non-zero.
	// Alternatively we could probably mine a block.
	if e.wallet.Balance(false) == 0 {
		t.Error("e.wallet is empty.")
		return
	}

	// Send all coins to the `1` address.
	dest := consensus.CoinAddress{1}
	txn, err := e.SpendCoins(e.wallet.Balance(false)-10, dest)
	if err != nil {
		t.Error(err)
		return
	}
	err = e.processTransaction(txn)
	if err != nil {
		t.Error(err)
	}

	// Check that the transaction made it into the transaction pool.
	if len(e.state.TransactionPoolDump()) != 1 {
		t.Error("transaction pool not len 1", len(e.state.TransactionPoolDump()))
	}

	// Check that the balance of e.wallet.Balance(false) has dropped to 0.
	if e.wallet.Balance(false) != 0 {
		t.Error("wallet.Balance(false) should be 0, but instead is", e.wallet.Balance(false))
	}

	// Mine the block and see if the outputs moved.
	mineSingleBlock(t, e)
	sortedSet := e.state.SortedUtxoSet()
	if len(sortedSet) != 3 {
		t.Error(sortedSet)
		t.Fatal("expecting sortedSet to be len 3, got", len(sortedSet))
	}

	// At least one of the outputs should belong to address `1`.
	if sortedSet[0].SpendHash != dest &&
		sortedSet[1].SpendHash != dest &&
		sortedSet[2].SpendHash != dest {
		t.Error("no outputs belong to the transaction destination")
		t.Error(sortedSet[0].SpendHash, "\n", sortedSet[1].SpendHash, "\n", sortedSet[2].SpendHash)
	}
	// At least one of the outputs should belong to empty (the genesis).
	genesisAddress := consensus.CoinAddress{}
	if sortedSet[0].SpendHash != genesisAddress &&
		sortedSet[1].SpendHash != genesisAddress &&
		sortedSet[2].SpendHash != genesisAddress {
		t.Error("no outputs belong to genesis address")
	}

	// Check that the full wallet balance is reporting to only have the miner
	// subsidy.
	minerSubsidy := consensus.CalculateCoinbase(e.Height())
	minerSubsidy += 10 // TODO: Wallet figures out miner fee.
	if e.wallet.Balance(true) != minerSubsidy {
		t.Errorf("full balance not reporting correctly, should be %v but instead is %v", minerSubsidy, e.wallet.Balance(true))
		return
	}
}
