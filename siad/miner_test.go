package main

import (
	"testing"
)

// mineSingleBlock mines a single block and then uses the blocking function
// processBlock to integrate the block with the state.
func mineSingleBlock(t *testing.T, d *daemon) {
	_, found, err := d.miner.SolveBlock()
	for !found && err == nil {
		_, found, err = d.miner.SolveBlock()
	}
	if err != nil {
		t.Error(err)
	}
}

func testMinerDeadlocking(t *testing.T, d *daemon) {
	d.miner.Threads()
	d.miner.SetThreads(2)
	d.miner.StartMining()
	d.miner.Threads()
	d.miner.StopMining()
	d.miner.Threads()
}
