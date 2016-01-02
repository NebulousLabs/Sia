package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

func TestGatewayStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayStatus")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
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
	defer st.server.Close()
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerAdd", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdPostAPI("/gateway/peers/add/"+string(peer.Address()), nil)

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
	defer st.server.Close()
	peer, err := gateway.New(":0", build.TempDir("api", "TestGatewayPeerRemove", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdPostAPI("/gateway/peers/add/"+string(peer.Address()), nil)

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peers/add did not add peer", peer.Address())
	}

	st.stdPostAPI("/gateway/peers/remove/"+string(peer.Address()), nil)
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}
}
