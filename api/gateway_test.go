package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// addPeer creates a new serverTester and bootstraps it to st. It returns the
// new peer.
func (st *serverTester) addPeer(name string) (*serverTester, error) {
	_, err := st.miner.AddBlock()
	if err != nil {
		return nil, err
	}

	// Create a new peer and bootstrap it to st.
	newPeer, err := createServerTester(name)
	if err != nil {
		return nil, err
	}
	err = newPeer.server.gateway.Connect(st.netAddress())
	if err != nil {
		return nil, err
	}
	return newPeer, nil
}

func TestGatewayStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayStatus")
	if err != nil {
		t.Fatal(err)
	}
	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/status gave bad peer list:", info.Peers)
	}
}

func TestGatewayPeerAdd(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerAdd")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerAdd", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdGetAPI("/gateway/peers/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peers/add did not add peer", peer.Address())
	}
}

func TestGatewayPeerRemove(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerRemove")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerRemove", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdGetAPI("/gateway/peers/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peers/add did not add peer", peer.Address())
	}

	st.stdGetAPI("/gateway/peers/remove?address=" + string(peer.Address()))
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}
}
