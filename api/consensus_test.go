package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestConsensusGet probes the GET call to /consensus.
func TestConsensusGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st := newServerTester("TestConsensusGET", t)
	var css ConsensusSetStatus
	st.getAPI("/consensus", &css) // TODO: err =
	if css.Height != 4 {
		t.Error("wrong height returned in consensus GET call")
	}
	if css.CurrentBlock != st.server.currentBlock.ID() {
		t.Error("wrong block returned in consensus GET call")
	}
	expectedTarget := types.Target{64}
	if css.Target != expectedTarget {
		t.Error("wrong target returned in consensus GET call")
	}
}

// TestConsensusSynchronizeGET probes the GET call to /consensus/synchronize.
func TestConsensusSynchronizeGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st := newServerTester("TestConsensusSynchronizeGET", t)
	st.callAPI("/consensus/synchronize") // TODO: err = 

	// TODO: Need some way to tell that a peer was out of sync, and then
	// in-sync. The problem is that currently, if there are peers they should
	// synchronize automatically. /consensus/synchronize is much closer to a
	// debugging api call than an actual api call.
}
