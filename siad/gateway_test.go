package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// addPeer creates a new daemonTester and bootstraps it to dt. It returns the
// new peer.
func (dt *daemonTester) addPeer() *daemonTester {
	// Create a new peer and bootstrap it to dt.
	newPeer := newDaemonTester(dt.T)
	err := newPeer.gateway.Bootstrap(dt.address())
	if err != nil {
		dt.Fatal("bootstrap failed:", err)
	}

	// Wait for RPC to finish, then check that each has the same number of
	// peers.
	<-dt.rpcChan
	if len(dt.gateway.Info().Peers) != len(newPeer.gateway.Info().Peers) {
		dt.Fatal("bootstrap did not synchronize peer lists")
	}
	return newPeer
}

// TestPeering tests that peers are properly announced and relayed throughout
// the network.
func TestPeering(t *testing.T) {
	// Create to peers and add the first to the second.
	peer1 := newDaemonTester(t)
	peer2 := newDaemonTester(t)
	peer1.callAPI("/gateway/add?addr=" + string(peer2.address()))

	// Check that the first has the second as a peer.
	var info modules.GatewayInfo
	peer1.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer2.address() {
		t.Fatal("/gateway/add did not add peer", peer2.address())
	}

	// Create a third peer that bootstraps to the first peer and check that it
	// reports the others as peers.
	peer3 := peer1.addPeer()
	peer3.getAPI("/gateway/status", &info)
	if len(info.Peers) != 2 {
		t.Fatal("bootstrap peer did not share its peers")
	}

	// peer2 should have received peer3 via peer1. Note that it does not have
	// peer1 though, because /gateway/add does not contact the added peer.
	peer2.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer3.address() {
		t.Fatal("bootstrap peer did not relay the bootstrapping peer")
	}
}
