package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// TestGatewayStatus checks that the /gateway/status call is returning a corect
// peerlist.
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
	st.getAPI("/gateway", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway gave bad peer list:", info.Peers)
	}
}

// TestGatewayPeerAdd checks that /gateway/add is adding a peer to the
// gateway's peerlist.
func TestGatewayPeerAdd(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerConnect")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	peer, err := gateway.New("localhost:0", build.TempDir("api", "TestGatewayPeerConnect", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdPostAPI("/gateway/connect/"+string(peer.Address()), nil)

	var info GatewayInfo
	st.getAPI("/gateway", &info)
	if len(info.Peers) != 1 || info.Peers[0].NetAddress != peer.Address() {
		t.Fatal("/gateway/connect did not connect to peer", peer.Address())
	}
}

// TestGatewayPeerRemove checks that gateway/remove removes the correct peer
// from the gateway's peerlist.
func TestGatewayPeerRemove(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerDisconnect")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	peer, err := gateway.New("localhost:0", build.TempDir("api", "TestGatewayPeerDisconnect", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.stdPostAPI("/gateway/connect/"+string(peer.Address()), nil)

	var info GatewayInfo
	st.getAPI("/gateway", &info)
	if len(info.Peers) != 1 || info.Peers[0].NetAddress != peer.Address() {
		t.Fatal("/gateway/connect did not connect to peer", peer.Address())
	}

	st.stdPostAPI("/gateway/disconnect/"+string(peer.Address()), nil)
	st.getAPI("/gateway", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/disconnect did not disconnect from peer", peer.Address())
	}
}
