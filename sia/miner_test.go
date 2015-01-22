package sia

import (
	"testing"
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
	t.Error("Submitting block:", b.ID())
}

func testMinerDeadlocking(t *testing.T, c *Core) {
	c.miner.Threads()
	c.miner.SetThreads(2)
	c.miner.StartMining()
	c.miner.Threads()
	c.miner.StopMining()
	c.miner.Threads()
}
