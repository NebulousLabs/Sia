package gateway

import (
	//"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
)

// TestNodeSharing tests that nodes are correctly shared.
func TestNodeSharing(t *testing.T) {
	g1 := newTestingGateway("TestPeerSharing1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestPeerSharing2", t)
	defer g2.Close()
	peer, err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// ask gateway for nodes
	var nodes []modules.NetAddress
	err = peer.rpc("ShareNodes", readerRPC(&nodes, 1024))
	if err != nil {
		t.Fatal(err)
	}
	// response should be exactly []NetAddress{g1.Address(), g2.Address()}
	if len(nodes) != 2 || (nodes[0] != g1.Address() && nodes[1] != g1.Address()) {
		t.Fatalf("gateway gave bad node list %v (expected %v)", nodes, []modules.NetAddress{g1.Address(), g2.Address()})
	}

	// add a couple more nodes
	g2.addNode("foo:9001")
	g2.addNode("bar:9002")
	g2.addNode("baz:9003")
	err = peer.rpc("ShareNodes", readerRPC(&nodes, 1024))
	if err != nil {
		t.Fatal(err)
	}
	// nodes should now contain 4 distinct addresses
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i] == nodes[j] {
				t.Fatal("gateway gave duplicate addresses:", nodes)
			}
		}
	}

	// remove all the nodes
	g2.removeNode("foo:9001")
	g2.removeNode("bar:9002")
	g2.removeNode("baz:9003")
	g2.removeNode(g1.Address())
	g2.removeNode(g2.Address())
	if len(g2.nodes) != 0 {
		t.Fatalf("gateway has %d node(s) remaining after removal", g2.Info().Nodes)
	}

	// no nodes should be returned
	err = peer.rpc("ShareNodes", readerRPC(&nodes, 1024))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatal("gateway gave non-existent addresses:", nodes)
	}
}

/*

// TestBadPeer tests that "bad" peers are correctly identified and removed.
// TODO: bring back strike system
func TestBadPeer(t *testing.T) {
	g := newTestingGateway("TestBadPeer1", t)
	defer g.Close()

	// create bad peer
	badpeer := newTestingGateway("TestBadPeer2", t)
	// overwrite badpeer's Ping RPC with an incorrect one
	badpeer.RegisterRPC("Ping", writerRPC("lol"))

	g.addNode(badpeer.Address())

	// try to ping the peer 'maxStrikes'+1 times
	for i := 0; i < maxStrikes+1; i++ {
		g.Ping(badpeer.Address())
	}

	// since we are poorly-connected, badpeer should still be in our peer list
	if len(g.peers) != 1 {
		t.Fatal("gateway removed peer when poorly-connected:", g.Info().Peers)
	}

	// add minPeers more peers
	for i := 0; i < minPeers; i++ {
		g.addNode(modules.NetAddress("foo" + strconv.Itoa(i)))
	}

	// once we exceed minPeers, badpeer should be kicked out
	if len(g.peers) != minPeers {
		t.Fatal("gateway did not remove bad peer after becoming well-connected:", g.Info().Peers)
	} else if _, ok := g.peers[badpeer.Address()]; ok {
		t.Fatal("gateway removed wrong peer:", g.Info().Peers)
	}
}

*/

// TestBootstrap tests the bootstrapping process, including synchronization.
func TestBootstrap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create bootstrap peer
	bootstrap := newTestingGateway("TestBootstrap1", t)
	ct := consensus.NewConsensusTester(t, bootstrap.state)
	// give it some blocks
	for i := 0; i < MaxCatchUpBlocks*2+1; i++ {
		ct.MineAndApplyValidBlock()
	}
	// give it a peer
	bootstrap.Connect(newTestingGateway("TestBootstrap2", t).Address())

	// bootstrap a new peer
	g := newTestingGateway("TestBootstrap3", t)
	err := g.Bootstrap(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}

	// wait for synchronize to complete
	for g.state.Height() != bootstrap.state.Height() {
		time.Sleep(10 * time.Millisecond)
	}

	// node lists should be the same
	if g.Info().Nodes != bootstrap.Info().Nodes {
		t.Fatalf("gateway peer list %v does not match bootstrap peer list %v", g.nodes, bootstrap.nodes)
	}
}
