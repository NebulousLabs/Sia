package gateway

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// newTestingGateway returns a gateway read to use in a testing environment.
func newTestingGateway(name string, t *testing.T) *Gateway {
	g, err := New(":0", build.TempDir("gateway", name))
	if err != nil {
		t.Fatal(err)
	}
	// Manually add myAddr as a node. This is necessary because g.addNode
	// rejects loopback addresses.
	id := g.mu.Lock()
	g.nodes[g.myAddr] = struct{}{}
	g.mu.Unlock(id)
	return g
}

func TestAddress(t *testing.T) {
	g := newTestingGateway("TestAddress", t)
	defer g.Close()
	if g.Address() != g.myAddr {
		t.Fatal("Address does not return g.myAddr")
	}
	port := modules.NetAddress(g.listener.Addr().String()).Port()
	expAddr := modules.NetAddress(net.JoinHostPort("::", port))
	if g.Address() != expAddr {
		t.Fatalf("Wrong address: expected %v, got %v", expAddr, g.Address())
	}
}

func TestPeers(t *testing.T) {
	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRPC2", t)
	defer g2.Close()
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}
	peers := g1.Peers()
	if len(peers) != 1 || peers[0] != g2.Address() {
		t.Fatal("g1 has bad peer list:", peers)
	}
	err = g1.Disconnect(g2.Address())
	if err != nil {
		t.Fatal("failed to disconnect:", err)
	}
	peers = g1.Peers()
	if len(peers) != 0 {
		t.Fatal("g1 has peers after disconnect:", peers)
	}
}

func TestNew(t *testing.T) {
	if _, err := New("", ""); err == nil {
		t.Fatal("expecting persistDir error, got nil")
	}
	if _, err := New(":0", ""); err == nil {
		t.Fatal("expecting persistDir error, got nil")
	}
	if g, err := New("foo", build.TempDir("gateway", "TestNew1")); err == nil {
		t.Fatal("expecting listener error, got nil", g.myAddr)
	}
	// create corrupted nodes.json
	dir := build.TempDir("gateway", "TestNew2")
	os.MkdirAll(dir, 0700)
	err := ioutil.WriteFile(filepath.Join(dir, "nodes.json"), []byte{1, 2, 3}, 0660)
	if err != nil {
		t.Fatal("couldn't create corrupted file:", err)
	}
	if _, err := New(":0", dir); err == nil {
		t.Fatal("expected load error, got nil")
	}
}
