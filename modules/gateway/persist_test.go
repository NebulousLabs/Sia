package gateway

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

func TestLoad(t *testing.T) {
	g := newTestingGateway("TestLoad", t)
	id := g.mu.Lock()
	g.addNode("foo")
	g.save()
	g.mu.Unlock(id)
	g.Close()

	g2, err := New(":0", g.saveDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g2.nodes["foo"]; !ok {
		t.Fatal("gateway did not load old peer list:", g2.nodes)
	}
}

func TestLoadPeer(t *testing.T) {
	g1 := newTestingGateway("TestLoadPeer1", t)
	g2 := newTestingGateway("TestLoadPeer2", t)

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("couldn't connect")
	}
	// g2 is now in g1's node and peer list
	g1.Close()

	// g1 should reconnect to g2 upon load
	g1, err = New(":0", g1.saveDir)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	peers := g1.Peers()
	if len(peers) != 1 || peers[0] != g2.Address() {
		t.Fatalf("gateway did not reconnect to loaded peer: expected %v, got %v", []modules.NetAddress{g2.Address()}, peers)
	}
}
