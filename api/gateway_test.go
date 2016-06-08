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

// TestGatewayPeerConnect checks that /gateway/connect is adding a peer to the
// gateway's peerlist.
func TestGatewayPeerConnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerConnect1")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	peer, err := gateway.New("localhost:0", build.TempDir("api", "TestGatewayPeerConnect2", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	err = st.stdPostAPI("/gateway/connect/"+string(peer.Address()), nil)
	if err != nil {
		t.Fatal(err)
	}

	var info GatewayInfo
	err = st.getAPI("/gateway", &info)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Peers) != 1 || info.Peers[0].NetAddress != peer.Address() {
		t.Fatal("/gateway/connect did not connect to peer", peer.Address())
	}
}

// TestGatewayPeerDisconnect checks that /gateway/disconnect removes the
// correct peer from the gateway's peerlist.
func TestGatewayPeerDisconnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestGatewayPeerDisconnect1")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	peer, err := gateway.New("localhost:0", build.TempDir("api", "TestGatewayPeerDisconnect2", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	err = st.stdPostAPI("/gateway/connect/"+string(peer.Address()), nil)
	if err != nil {
		t.Fatal(err)
	}

	var info GatewayInfo
	st.getAPI("/gateway", &info)
	if len(info.Peers) != 1 || info.Peers[0].NetAddress != peer.Address() {
		t.Fatal("/gateway/connect did not connect to peer", peer.Address())
	}

	err = st.stdPostAPI("/gateway/disconnect/"+string(peer.Address()), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = st.getAPI("/gateway", &info)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/disconnect did not disconnect from peer", peer.Address())
	}
}
