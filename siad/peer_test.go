package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// addPeer creates a new daemonTester and bootstraps it to dt. It returns the
// new peer.
func (dt *daemonTester) addPeer() *daemonTester {
	newPeer := newDaemonTester(dt.T)
	// bootstrap newPeer to dt
	err := newPeer.gateway.Bootstrap(dt.Address())
	if err != nil {
		dt.Fatal("bootstrap failed:", err)
	}
	// newPeer and dt should now have the same number of peers
	if len(dt.gateway.Info().Peers) != len(newPeer.gateway.Info().Peers) {
		dt.Fatal("bootstrap did not synchronize peer lists")
	}
	return newPeer
}

// TestPeering tests that peers are properly announced and relayed throughout
// the network.
func TestPeering(t *testing.T) {
	// create two peers
	peer1 := newDaemonTester(t)
	peer2 := newDaemonTester(t)
	// add peer2 to peer1
	peer1.callAPI("/peer/add?addr=" + string(peer2.Address()))
	// peer2 should now be in peer1's peer list
	var info modules.GatewayInfo
	peer1.getAPI("/peer/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer2.Address() {
		t.Fatal("/peer/add did not add peer", peer2.Address())
	}

	// now create a new peer that bootstraps to peer1
	peer3 := peer1.addPeer()
	// peer3 should have both peer1 and peer2
	peer3.getAPI("/peer/status", &info)
	if len(info.Peers) != 2 {
		t.Fatal("bootstrap peer did not share its peers")
	}

	// peer2 should have received peer3 via peer1. Note that it does not have
	// peer1 though, because /peer/add does not contact the added peer.
	peer2.getAPI("/peer/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer3.Address() {
		t.Fatal("bootstrap peer did not relay the bootstrapping peer")
	}
}
