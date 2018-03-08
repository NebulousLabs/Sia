package gateway

import (
	"fmt"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// TestIpRPC tests the ip discovery RPC.
func TestIpRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create gateways for testing.
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()

	// Connect gateways.
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}

	// Call RPC
	err = g1.RPC(g2.Address(), "DiscoverIP", func(conn modules.PeerConn) error {
		var address string
		err := encoding.ReadObject(conn, &address, 100)
		if err != nil {
			t.Error("failed to read object from response", err)
		}
		if address != g1.Address().Host() {
			return fmt.Errorf("ip addresses don't match %v != %v", g1.Address().Host(), address)
		}
		return nil
	})
	if err != nil {
		t.Fatal("RPC failed", err)
	}
}

// TestIpFromPeers test the functionality of managedIPFromPeers.
func TestIPFromPeers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create gateways for testing.
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()
	g3 := newNamedTestingGateway(t, "3")
	defer g2.Close()

	// Connect gateways.
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = g1.Connect(g3.Address())
	if err != nil {
		t.Fatal(err)
	}

	// Discover ip using the peers
	host, err := g1.managedIPFromPeers()
	if err != nil {
		t.Fatal("failed to get ip", err)
	}
	if host != g1.Address().Host() {
		t.Fatalf("ip should be %v but was %v", g1.Address().Host(), host)
	}
}
