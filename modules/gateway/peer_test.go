package gateway

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestPeerSharing tests that peers are correctly shared.
func TestPeerSharing(t *testing.T) {
	g := newTestGateway(t)

	// add a peer
	peer := modules.NetAddress("foo:9001")
	g.addPeer(peer)
	// gateway only has one peer, so randomPeer should return peer
	if p, err := g.randomPeer(); err != nil || p != peer {
		t.Fatal("gateway has bad peer list:", g.Info())
	}

	// ask gateway for peers
	var peers []modules.NetAddress
	err := g.RPC(g.myAddr, "SharePeers", modules.ReaderRPC(&peers, 1024))
	if err != nil {
		t.Fatal(err)
	}
	// response should be exactly []Address{peer}
	if len(peers) != 1 || peers[0] != peer {
		t.Fatal("gateway gave bad peer list:", peers)
	}

	// add a couple more peers
	g.addPeer("bar:9002")
	g.addPeer("baz:9003")
	g.addPeer("quux:9004")
	err = g.RPC(g.myAddr, "SharePeers", modules.ReaderRPC(&peers, 1024))
	if err != nil {
		t.Fatal(err)
	}
	// peers should now contain 4 distinct addresses
	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			if peers[i] == peers[j] {
				t.Fatal("gateway gave duplicate addresses:", peers)
			}
		}
	}
}

// TestBadPeer tests that "bad" peers are correctly identified and removed.
func TestBadPeer(t *testing.T) {
	g := newTestGateway(t)

	// create bad peer
	badpeer := newTestGateway(t)
	// overwrite badpeer's Ping RPC with an incorrect one
	badpeer.RegisterRPC("Ping", modules.WriterRPC("lol"))

	g.addPeer(badpeer.Address())

	// try to ping the peer 'maxStrikes'+1 times
	for i := 0; i < maxStrikes+1; i++ {
		g.Ping(badpeer.Address())
	}

	// badpeer should no longer be in our peer list
	if len(g.peers) != 0 {
		t.Fatal("gateway did not remove bad peer:", g.Info().Peers)
	}
}
