package siacore

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testEmptyBlock creates an emtpy block and submits it to the state.
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
	if e.wallet.Balance(false) == 0 {
		t.Error("e.wallet is empty.")
		return
	}

	// Send all coins to the `1` address.
	dest := consensus.CoinAddress{1}
	_, err := e.SpendCoins(e.wallet.Balance(false), dest)
	if err != nil {
		t.Error(err)
		return
	}

	// Mine the block and see if the outputs moved.
	mineSingleBlock(t, e)
	sortedSet := e.state.SortedUtxoSet()
	if len(sortedSet) != 2 {
		t.Error("expecting sortedSet to be len 2, got", len(sortedSet))
		return
	}

	// At least one of the outputs should belong to address `1`.
	if sortedSet[0].SpendHash != dest && sortedSet[1].SpendHash != dest {
		t.Error("neither of the outputs is the correct output.")
		return
	}
}
