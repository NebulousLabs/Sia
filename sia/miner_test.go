package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, c *Core) {
	b, found, err := c.miner.SolveBlock()
	for !found && err == nil {
		b, found, err = c.miner.SolveBlock()
	}
	if err != nil {
		t.Error(err)
	}

	// TODO: Depricate
	err = c.processBlock(b)
	if err != nil && err != consensus.BlockKnownErr { // TODO: depricate
		t.Error(err)
	}
}

func testMinerDeadlocking(t *testing.T, c *Core) {
	c.miner.Threads()
	c.miner.SetThreads(2)
	c.miner.StartMining()
	c.miner.Threads()
	c.miner.StopMining()
	c.miner.Threads()
}
