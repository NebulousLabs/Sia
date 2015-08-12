package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// addPeer creates a new serverTester and bootstraps it to st. It returns the
// new peer.
func (st *serverTester) addPeer(name string) *serverTester {
	b, _ := st.miner.FindBlock()
	err := st.cs.AcceptBlock(b)
	if err != nil {
		st.t.Fatal(err)
	}

	// Create a new peer and bootstrap it to st.
	newPeer := newServerTester(name, st.t)
	err = newPeer.server.gateway.Connect(st.netAddress())
	if err != nil {
		st.t.Fatal("bootstrap failed:", err)
	}
	return newPeer
}

func TestGatewayStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st := newServerTester("TestGatewayStatus", t)
	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/status gave bad peer list:", info.Peers)
	}
}

func TestGatewayPeerAdd(t *testing.T) {
	st := newServerTester("TestGatewayPeerAdd", t)
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerAdd", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.callAPI("/gateway/peers/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peers/add did not add peer", peer.Address())
	}
}

func TestGatewayPeerRemove(t *testing.T) {
	st := newServerTester("TestGatewayPeerRemove", t)
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerRemove", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.callAPI("/gateway/peers/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peers/add did not add peer", peer.Address())
	}

	st.callAPI("/gateway/peers/remove?address=" + string(peer.Address()))
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}
}
