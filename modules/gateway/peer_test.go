package gateway

import (
	"testing"

	"github.com/NebulousLabs/Sia/network"
)

func TestPeerSharing(t *testing.T) {
	// create server
	tcps, err := network.NewTCPServer(":9002")
	if err != nil {
		t.Fatal(err)
	}
	myAddr := tcps.Address()
	g := New(tcps, nil, nil)

	// register SharePeers RPC
	tcps.RegisterRPC("SharePeers", g.SharePeers)

	// add a peer
	peer := network.Address("foo:9001")
	g.addPeer(peer)
	// gateway only has one peer, so randomPeer should return peer
	if p, err := g.randomPeer(); err != nil || p != peer {
		t.Fatal("server has bad peer list:", g.Info())
	}

	// ask tcps for peers
	var resp []network.Address
	err = myAddr.RPC("SharePeers", nil, &resp)
	if err != nil {
		t.Fatal(err)
	}
	// resp should be exactly []Address{peer}
	if len(resp) != 1 || resp[0] != peer {
		t.Fatal("server gave bad peer list:", resp)
	}

	// add a couple more peers
	g.addPeer(network.Address("bar:9002"))
	g.addPeer(network.Address("baz:9003"))
	g.addPeer(network.Address("quux:9004"))
	err = myAddr.RPC("SharePeers", nil, &resp)
	if err != nil {
		t.Fatal(err)
	}
	// resp should now contain 4 distinct addresses
	for i := 0; i < len(resp); i++ {
		for j := i + 1; j < len(resp); j++ {
			if resp[i] == resp[j] {
				t.Fatal("resp contains duplicate addresses:", resp)
			}
		}
	}
}
