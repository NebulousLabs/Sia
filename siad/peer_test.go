package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestPeering tests that peers are properly announced and relayed throughout
// the network.
func TestPeering(t *testing.T) {
	// create three peers
	peer1 := newDaemonTester(t)
	peer2 := newDaemonTester(t)
	peer3 := newDaemonTester(t)
	// add peer3 to peer2
	peer2.callAPI("/peer/add?addr=" + string(peer3.Address()))
	// peer3 should now be in peer2's peer list
	var info modules.GatewayInfo
	peer2.getAPI("/peer/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer3.Address() {
		t.Fatal("/peer/add did not add peer", peer3.Address())
	}
	// have peer1 bootstrap to peer2
	err := peer1.gateway.Bootstrap(peer2.Address())
	if err != nil {
		t.Fatal("bootstrap failed:", err)
	}
	// peer1 should now have both peer2 and peer3
	peer1.getAPI("/peer/status", &info)
	if len(info.Peers) != 2 {
		t.Fatal("bootstrap peer did not share its peers")
	}
	// peer3 should have received peer1 via peer2. Note that it does not have
	// peer2 though, because peer2 did not use the "AddMe" RPC.
	peer3.getAPI("/peer/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer1.Address() {
		t.Fatal("bootstrap peer did not relay the bootstrapping peer")
	}
}
