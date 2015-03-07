package gateway

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// TestPeerSharing tests that peers are correctly shared.
func TestPeerSharing(t *testing.T) {
	g := newTestGateway(t)
	defer g.Close()

	// add a peer
	peer := modules.NetAddress("foo:9001")
	g.AddPeer(peer)
	// gateway only has one peer, so randomPeer should return peer
	if p, err := g.randomPeer(); err != nil || p != peer {
		t.Fatal("gateway has bad peer list:", g.Info().Peers)
	}

	// ask gateway for peers
	var peers []modules.NetAddress
	err := g.RPC(g.myAddr, "SharePeers", readerRPC(&peers, 1024))
	if err != nil {
		t.Fatal(err)
	}
	// response should be exactly []Address{peer}
	if len(peers) != 1 || peers[0] != peer {
		t.Fatal("gateway gave bad peer list:", peers)
	}

	// add a couple more peers
	g.AddPeer("bar:9002")
	g.AddPeer("baz:9003")
	g.AddPeer("quux:9004")
	err = g.RPC(g.myAddr, "SharePeers", readerRPC(&peers, 1024))
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

	// remove all the peers
	g.RemovePeer("foo:9001")
	g.RemovePeer("bar:9002")
	g.RemovePeer("baz:9003")
	g.RemovePeer("quux:9004")
	if len(g.peers) != 0 {
		t.Fatal("gateway has peers remaining after removal:", g.Info().Peers)
	}

	// no peers should be returned
	err = g.RPC(g.myAddr, "SharePeers", readerRPC(&peers, 1024))
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatal("gateway gave non-existent addresses:", peers)
	}
}

// TestBadPeer tests that "bad" peers are correctly identified and removed.
func TestBadPeer(t *testing.T) {
	g := newTestGateway(t)
	defer g.Close()

	// create bad peer
	badpeer := newTestGateway(t)
	// overwrite badpeer's Ping RPC with an incorrect one
	badpeer.RegisterRPC("Ping", writerRPC("lol"))

	g.AddPeer(badpeer.Address())

	// try to ping the peer 'maxStrikes'+1 times
	for i := 0; i < maxStrikes+1; i++ {
		g.Ping(badpeer.Address())
	}

	// badpeer should no longer be in our peer list
	if len(g.peers) != 0 {
		t.Fatal("gateway did not remove bad peer:", g.Info().Peers)
	}
}

// TestBootstrap tests the bootstrapping process, including synchronization.
func TestBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// create bootstrap peer
	bootstrap := newTestGateway(t)
	ct := consensus.NewConsensusTester(t, bootstrap.state)
	// give it some blocks
	for i := 0; i < MaxCatchUpBlocks*2+1; i++ {
		ct.MineAndApplyValidBlock()
	}
	// give it a peer
	bootstrap.AddPeer(newTestGateway(t).Address())

	// bootstrap a new peer
	g := newTestGateway(t)
	err := g.Bootstrap(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}

	// heights should match
	if g.state.Height() != bootstrap.state.Height() {
		// g may have tried to synchronize to the other peer, so try manually
		// synchronizing to the bootstrap
		g.synchronize(bootstrap.Address())
		if g.state.Height() != bootstrap.state.Height() {
			t.Fatalf("gateway height %v does not match bootstrap height %v", g.state.Height(), bootstrap.state.Height())
		}
	}
	// peer lists should be the same size, though they won't match; bootstrap
	// will have g and g will have bootstrap.
	if len(g.Info().Peers) != len(bootstrap.Info().Peers) {
		t.Fatalf("gateway peer list %v does not match bootstrap peer list %v", g.Info().Peers, bootstrap.Info().Peers)
	}

	// add another two peers to bootstrap: a real peer and a "dummy", which won't respond.
	bootstrap.AddPeer(newTestGateway(t).Address())
	bootstrap.AddPeer("dummy")

	// have g request peers from bootstrap. g should add the real peer, but not the dummy.
	err = g.requestPeers(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Info().Peers) != len(bootstrap.Info().Peers)-1 {
		t.Fatal("gateway added wrong peers:", g.Info().Peers)
	}
}
