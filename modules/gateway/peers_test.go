package gateway

import (
	//"strconv"
	"net"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	//	"github.com/NebulousLabs/Sia/modules"
)

func TestAddPeer(t *testing.T) {
	g := newTestingGateway("TestAddPeer", t)
	defer g.Close()
	g.addPeer("foo", nil)
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
	if len(g.nodes) != 2 {
		t.Fatal("gateway did not add node")
	}
}

func TestListen(t *testing.T) {
	g := newTestingGateway("TestListen", t)
	defer g.Close()
	// "compliant" connect
	conn, err := net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	if err := encoding.WriteObject(conn, "foo"); err != nil {
		t.Fatal("couldn't write address")
	}
	// g should have added foo
	/*
		if g.peers["foo"] == nil {
			t.Fatal("g did not add connecting node")
		}
	*/
	defer conn.Close()
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
