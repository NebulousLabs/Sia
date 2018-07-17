package gateway

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/modules"
	siasync "gitlab.com/NebulousLabs/Sia/sync"
)

// newTestingGateway returns a gateway ready to use in a testing environment.
func newTestingGateway(t *testing.T) *Gateway {
	if testing.Short() {
		panic("newTestingGateway called during short test")
	}

	g, err := New("localhost:0", false, build.TempDir("gateway", t.Name()))
	if err != nil {
		panic(err)
	}
	return g
}

// newNamedTestingGateway returns a gateway ready to use in a testing
// environment. The gateway's persist folder will have the specified suffix.
func newNamedTestingGateway(t *testing.T, suffix string) *Gateway {
	if testing.Short() {
		panic("newTestingGateway called during short test")
	}

	g, err := New("localhost:0", false, build.TempDir("gateway", t.Name()+suffix))
	if err != nil {
		panic(err)
	}
	return g
}

// TestExportedMethodsErrAfterClose tests that exported methods like Close and
// Connect error with siasync.ErrStopped after the gateway has been closed.
func TestExportedMethodsErrAfterClose(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)

	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != siasync.ErrStopped {
		t.Fatalf("expected %q, got %q", siasync.ErrStopped, err)
	}
	if err := g.Connect("localhost:1234"); err != siasync.ErrStopped {
		t.Fatalf("expected %q, got %q", siasync.ErrStopped, err)
	}
}

// TestAddress tests that Gateway.Address returns the address of its listener.
// Also tests that the address is not unspecified and is a loopback address.
// The address must be a loopback address for testing.
func TestAddress(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	if g.Address() != g.myAddr {
		t.Fatal("Address does not return g.myAddr")
	}
	if g.Address() != modules.NetAddress(g.listener.Addr().String()) {
		t.Fatalf("wrong address: expected %v, got %v", g.listener.Addr(), g.Address())
	}
	host := modules.NetAddress(g.listener.Addr().String()).Host()
	ip := net.ParseIP(host)
	if ip == nil {
		t.Fatal("address is not an IP address")
	}
	if ip.IsUnspecified() {
		t.Fatal("expected a non-unspecified address")
	}
	if !ip.IsLoopback() {
		t.Fatal("expected a loopback address")
	}
}

// TestPeers checks that two gateways are able to connect to each other.
func TestPeers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}
	peers := g1.Peers()
	if len(peers) != 1 || peers[0].NetAddress != g2.Address() {
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

// TestNew checks that a call to New is effective.
func TestNew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	if _, err := New("", false, ""); err == nil {
		t.Fatal("expecting persistDir error, got nil")
	}
	if _, err := New("localhost:0", false, ""); err == nil {
		t.Fatal("expecting persistDir error, got nil")
	}
	if g, err := New("foo", false, build.TempDir("gateway", t.Name()+"1")); err == nil {
		t.Fatal("expecting listener error, got nil", g.myAddr)
	}
	// create corrupted nodes.json
	dir := build.TempDir("gateway", t.Name()+"2")
	os.MkdirAll(dir, 0700)
	err := ioutil.WriteFile(filepath.Join(dir, "nodes.json"), []byte{1, 2, 3}, 0660)
	if err != nil {
		t.Fatal("couldn't create corrupted file:", err)
	}
	if _, err := New("localhost:0", false, dir); err == nil {
		t.Fatal("expected load error, got nil")
	}
}

// TestClose creates and closes a gateway.
func TestClose(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	g := newTestingGateway(t)
	err := g.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestParallelClose spins up 3 gateways, connects them all, and then closes
// them in parallel. The goal of this test is to make it more vulnerable to any
// potential nondeterministic failures.
func TestParallelClose(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Spin up three gateways in parallel.
	var gs [3]*Gateway
	var wg sync.WaitGroup
	wg.Add(3)
	for i := range gs {
		go func(i int) {
			gs[i] = newNamedTestingGateway(t, strconv.Itoa(i))
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Connect g1 to g2, g2 to g3. They may connect to eachother further.
	wg.Add(2)
	for i := range gs[:2] {
		go func(i int) {
			err := gs[i].Connect(gs[i+1].myAddr)
			if err != nil {
				panic(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Close all three gateways in parallel.
	wg.Add(3)
	for i := range gs {
		go func(i int) {
			err := gs[i].Close()
			if err != nil {
				panic(err)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}
