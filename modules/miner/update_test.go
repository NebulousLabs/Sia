package miner

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationThreadedConsensusRescan probes the threadedConsensusRescan
// function, checking that it works in the naive case.
func TestIntegrationThreadedConsensusRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester("TestIntegrationThreadedConsensusRescan")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the miner's persist variables have been initialized to the
	// first few blocks.
	if mt.miner.persist.RecentChange == (modules.ConsensusChangeID{}) || mt.miner.persist.Height == 0 || mt.miner.persist.Target == (types.Target{}) {
		t.Fatal("miner persist variables not initialized")
	}
	oldChange := mt.miner.persist.RecentChange
	oldHeight := mt.miner.persist.Height
	oldTarget := mt.miner.persist.Target

	// Corrupt the miner, the corruption should be fixed by the rescan.
	mt.miner.persist.RecentChange[0]++
	mt.miner.persist.Height += 500
	mt.miner.persist.Target[0]++

	// Call rescan and block until the scan is complete.
	c := make(chan error)
	go mt.miner.threadedConsensusRescan(c)
	err = <-c
	if err != nil {
		t.Fatal(err)
	}
	// Check that after rescanning, the values have returned to the usual values.
	if mt.miner.persist.RecentChange != oldChange {
		t.Error("rescan failed, ended up on the wrong change")
	}
	if mt.miner.persist.Height != oldHeight {
		t.Error("rescan failed, ended up at the wrong height")
	}
	if mt.miner.persist.Target != oldTarget {
		t.Error("rescan failed, ended up at the wrong target")
	}
}
