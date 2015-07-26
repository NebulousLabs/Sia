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
	if testing.Short() {
		t.SkipNow()
	}

	// Create a server tester and give it a peer. Get the peer ahead of the
	// server tester, then call 'synchronize' to bring them back to the same
	// height.
	st := newServerTester("TestConsensusSynchronizeGET", t)

	// Call synchronize when there are no peers.
	err := st.callAPI("/consensus/synchronize")
	if err == nil {
		t.Error("expecting an error - gateway has no peers")
	}

	// Mine a block so that when a peer is added, 'st' has a longer blockchain.
	block, _ := st.miner.FindBlock()
	err = st.cs.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	// Create a peer and bootstrap it to st.
	peer := newServerTester("TestConsensusSynchronizeGET - Peer", t)
	err = peer.server.gateway.Connect(st.netAddress())
	if err != nil {
		t.Fatal(err)
	}
	// Check that the heights are different on the consensus sets.
	if st.cs.CurrentBlock().ID() == peer.cs.CurrentBlock().ID() {
		// TODO: There is not a known way to add peers to a network without
		// them synchronizing automatically. Perhaps it is proof that
		// synchronize is not needed anymore, but I am hesitant to drop it
		// until we are more certian that it is not needed.
		//
		// t.Fatal("test objects are already synchronized - calling synchronize will not provide useful information")
	}

	// Call synchronize.
	err = st.callAPI("/consensus/synchronize")
	if err != nil {
		t.Fatal(err)
	}
	if st.cs.CurrentBlock().ID() != peer.cs.CurrentBlock().ID() {
		t.Fatal("synchronization failed")
	}
}
