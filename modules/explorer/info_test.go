package explorer

import (
	"testing"
)

// TestStatistics grabs the statistics object from the block explorer and
// verifies that the data has been filled out.
func TestStatistics(t *testing.T) {
	et, err := createExplorerTester("TestStatistics")
	if err != nil {
		t.Fatal(err)
	}

	stats := et.explorer.Statistics()
	if stats.Height != e.blockchainHeight || e.blockchainHeight == 0 {
		t.Error("wrong height reported in stats object")
	}
	if stats.TransactionCount != e.transactionCount || e.transactionCount == 0 {
		t.Error("wrong transaction count reported in stats object")
	}
}

// TestBlock probes the Block function of the explorer.
func TestBlock(t *testing.T) {
	gb := et.cs.GenesisBlock()
	gbFetch, height, exists := et.explorer.Block(gb.ID())
	if !exists || height != 0 || gbFetch.ID() != gb.ID() {
		t.Error("call to 'Block' inside explorer failed")
	}
}
