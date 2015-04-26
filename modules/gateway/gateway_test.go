package gateway

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/tester"
)

// newTestingGateway returns a gateway read to use in a testing environment.
func newTestingGateway(name string, t *testing.T) *Gateway {
	g, err := New(":0", tester.TempDir("gateway", name))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestAddress(t *testing.T) {
	g := newTestingGateway("TestAddress", t)
	defer g.Close()
	if g.Address() != g.myAddr {
		t.Fatal("Address does not return g.myAddr")
	}
	port := modules.NetAddress(g.listener.Addr().String()).Port()
	expAddr := modules.NetAddress(net.JoinHostPort("::1", port))
	if g.Address() != expAddr {
		t.Fatalf("Wrong address: expected %v, got %v", expAddr, g.Address())
	}
}

func TestPeers(t *testing.T) {
	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRPC2", t)
	defer g1.Close()
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

func TestBootstrap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create bootstrap peer
	bootstrap := newTestingGateway("TestBootstrap1", t)

	// give it a peer
	err := bootstrap.Connect(newTestingGateway("TestBootstrap2", t).Address())
	if err != nil {
		t.Fatal("couldn't connect:", err)
	}

	// bootstrap a new peer
	g := newTestingGateway("TestBootstrap3", t)
	err = g.Bootstrap(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}

	// node lists should be the same
	if len(g.nodes) != len(bootstrap.nodes) {
		t.Fatalf("gateway peer list %v does not match bootstrap peer list %v", g.nodes, bootstrap.nodes)
	}

	err = g.Disconnect(bootstrap.Address())
	if err != nil {
		t.Fatal("failed to disconnect:", err)
	}
}

func TestNew(t *testing.T) {
	if _, err := New("", ""); err == nil {
		t.Fatal("expecting saveDir error, got nil")
	}
	if _, err := New(":0", ""); err == nil {
		t.Fatal("expecting saveDir error, got nil")
	}
	if g, err := New("foo", tester.TempDir("gateway", "TestNew1")); err == nil {
		t.Fatal("expecting listener error, got nil", g.myAddr)
	}
	// create corrupted peers.dat
	dir := tester.TempDir("gateway", "TestNew2")
	os.MkdirAll(dir, 0700)
	err := ioutil.WriteFile(filepath.Join(dir, "nodes.dat"), []byte{1, 2, 3}, 0660)
	if err != nil {
		t.Fatal("couldn't create corrupted file:", err)
	}
	if _, err := New(":0", dir); err == nil {
		t.Fatal("expected load error, got nil")
	}
}

func TestExternalIP(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ip, err := getExternalIP()
	if err != nil {
		t.Fatal("couldn't determine external IP:", err)
	}
	if net.ParseIP(ip) == nil {
		t.Fatal("getExternalIP returned bad IP:", ip)
	}
}

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
