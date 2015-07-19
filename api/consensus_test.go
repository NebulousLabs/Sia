package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationConsensusGet probes the GET call to /consensus.
func TestIntegrationConsensusGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st := newServerTester("TestConsensusGET", t)
	var css ConsensusSetStatus
	err := st.getAPI("/consensus", &css)
	if err != nil {
		t.Fatal(err)
	}
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

// TestIntegrationConsensusSynchronizeGET probes the GET call to
// /consensus/synchronize.
func TestIntegrationConsensusSynchronizeGET(t *testing.T) {
	t.Skip("no known way to add peers without automatically performing a synchronize")
	if testing.Short() {
		t.SkipNow()
	}

	st := newServerTester("TestConsensusSynchronizeGET", t)
	err := st.callAPI("/consensus/synchronize")
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Need some way to tell that a peer was out of sync, and then
	// in-sync. The problem is that currently, if there are peers they should
	// synchronize automatically. /consensus/synchronize is much closer to a
	// debugging api call than an actual api call.
}
