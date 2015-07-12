package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// Mine a bunch of blocks, checking each time that the stored
// value agrees with consensus
func (et *explorerTester) testConsensusUpdates(t *testing.T) {
	// Clear the notification about the genesis block
	<-et.eUpdateChan

	// 20 here is arbitrary
	for i := types.BlockHeight(0); i < 20; i++ {
		b, _ := et.miner.FindBlock()
		err := et.cs.AcceptBlock(b)
		if err != nil {
			et.t.Fatal(err)
		}
		et.csUpdateWait()

		if et.explorer.currentBlock.ID() != et.cs.CurrentBlock().ID() {
			et.t.Fatal("Current block does not match consensus block")
		}
	}
}

func TestConsensusUpdates(t *testing.T) {
	et := createExplorerTester("TestExplorerConsensusUpdate", t)
	et.testConsensusUpdates(t)
}
