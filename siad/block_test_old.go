package main

/*
import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// testTransactionBlock creates a transaction and checks that it makes it into
// the utxo set.
func testTransactionBlock(t *testing.T, c *Core) {
	// As a prereq the balance of the wallet needs to be non-zero.
	// Alternatively we could probably mine a block.
	if c.wallet.Balance(false) == 0 {
		t.Error("c.wallet is empty.")
		return
	}

	// Send all coins to the `1` address.
	dest := consensus.CoinAddress{1}
	_, err := c.wallet.SpendCoins(c.wallet.Balance(false)-10, dest)
	if err != nil {
		t.Error(err)
		return
	}

	// Check that the transaction made it into the transaction pool.
	if len(c.state.TransactionPoolDump()) != 1 {
		t.Error("transaction pool not len 1", len(c.state.TransactionPoolDump()))
	}

	// Check that the balance of c.wallet.Balance(false) has dropped to 0.
	if c.wallet.Balance(false) != 0 {
		t.Error("wallet.Balance(false) should be 0, but instead is", c.wallet.Balance(false))
	}

	// Mine the block and see if the outputs moved.
	// mineSingleBlock(t, c)
	sortedSet := c.state.SortedUtxoSet()
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
	minerSubsidy := consensus.CalculateCoinbase(c.state.Height())
	minerSubsidy += 10 // TODO: Wallet figures out miner fee.
	if c.wallet.Balance(true) != minerSubsidy {
		t.Errorf("full balance not reporting correctly, should be %v but instead is %v", minerSubsidy, c.wallet.Balance(true))
		return
	}
}
*/
