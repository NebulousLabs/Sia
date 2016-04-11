package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestInsertHost tests the insertHost method, which also depends on the
// scanHostEntry method.
func TestInsertHost(t *testing.T) {
	hdb := bareHostDB()

	// invalid host should not be scanned
	var dbe modules.HostDBEntry
	dbe.NetAddress = "foo"
	hdb.insertHost(dbe)
	select {
	case <-hdb.scanPool:
		t.Error("invalid host was added to scan pool")
	case <-time.After(100 * time.Millisecond):
	}

	// valid host should be scanned
	dbe.NetAddress = "foo:1234"
	hdb.insertHost(dbe)
	select {
	case <-hdb.scanPool:
	case <-time.After(time.Second):
		t.Error("host was not scanned")
	}

	// duplicate host should not be scanned
	hdb.insertHost(dbe)
	select {
	case <-hdb.scanPool:
		t.Error("duplicate host was added to scan pool")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestActiveHosts tests the ActiveHosts method.
func TestActiveHosts(t *testing.T) {
	hdb := bareHostDB()

	// empty
	if hosts := hdb.ActiveHosts(); len(hosts) != 0 {
		t.Errorf("wrong number of hosts: expected %v, got %v", 0, len(hosts))
	}

	// with one host
	h1 := new(hostEntry)
	h1.NetAddress = "foo"
	hdb.activeHosts = map[modules.NetAddress]*hostNode{
		h1.NetAddress: &hostNode{hostEntry: h1},
	}
	if hosts := hdb.ActiveHosts(); len(hosts) != 1 {
		t.Errorf("wrong number of hosts: expected %v, got %v", 1, len(hosts))
	} else if hosts[0].NetAddress != h1.NetAddress {
		t.Errorf("ActiveHosts returned wrong host: expected %v, got %v", h1.NetAddress, hosts[0].NetAddress)
	}

	// with multiple hosts
	h2 := new(hostEntry)
	h2.NetAddress = "bar"
	hdb.activeHosts = map[modules.NetAddress]*hostNode{
		h1.NetAddress: &hostNode{hostEntry: h1},
		h2.NetAddress: &hostNode{hostEntry: h2},
	}
	if hosts := hdb.ActiveHosts(); len(hosts) != 2 {
		t.Errorf("wrong number of hosts: expected %v, got %v", 2, len(hosts))
	} else if hosts[0].NetAddress != h1.NetAddress && hosts[1].NetAddress != h1.NetAddress {
		t.Errorf("ActiveHosts did not contain an inserted host: %v (missing %v)", hosts, h1.NetAddress)
	} else if hosts[0].NetAddress != h2.NetAddress && hosts[1].NetAddress != h2.NetAddress {
		t.Errorf("ActiveHosts did not contain an inserted host: %v (missing %v)", hosts, h2.NetAddress)
	}
}

// TestAveragePrice tests the AveragePrice method, which also depends on the
// randomHosts method.
func TestAveragePrice(t *testing.T) {
	hdb := bareHostDB()

	// empty
	if avg := hdb.AveragePrice(); !avg.IsZero() {
		t.Error("average of empty hostdb should be zero:", avg)
	}

	// with one host
	h1 := new(hostEntry)
	h1.NetAddress = "foo"
	h1.ContractPrice = types.NewCurrency64(100)
	h1.weight = baseWeight
	hdb.insertNode(h1)
	if avg := hdb.AveragePrice(); avg.Cmp(h1.ContractPrice) != 0 {
		t.Error("average of one host should be that host's price:", avg)
	}

	// with two hosts
	h2 := new(hostEntry)
	h2.NetAddress = "bar"
	h2.ContractPrice = types.NewCurrency64(300)
	h2.weight = baseWeight
	hdb.insertNode(h2)
	if len(hdb.activeHosts) != 2 {
		t.Error("host was not added:", hdb.activeHosts)
	}
	if avg := hdb.AveragePrice(); avg.Cmp(types.NewCurrency64(200)) != 0 {
		t.Error("average of two hosts should be their sum/2:", avg)
	}
}

// TestIsOffline tests the IsOffline method.
func TestIsOffline(t *testing.T) {
	hdb := &HostDB{
		allHosts: map[modules.NetAddress]*hostEntry{
			"foo:1234": &hostEntry{online: true},
			"bar:1234": &hostEntry{online: false},
			"baz:1234": &hostEntry{online: true},
		},
		activeHosts: map[modules.NetAddress]*hostNode{
			"foo:1234": nil,
		},
		scanPool: make(chan *hostEntry),
	}

	tests := []struct {
		addr    modules.NetAddress
		offline bool
	}{
		{"foo:1234", false},
		{"bar:1234", true},
		{"baz:1234", false},
		{"quux:1234", false},
	}
	for _, test := range tests {
		if offline := hdb.IsOffline(test.addr); offline != test.offline {
			t.Errorf("IsOffline(%v) = %v, expected %v", test.addr, offline, test.offline)
		}
	}
}
