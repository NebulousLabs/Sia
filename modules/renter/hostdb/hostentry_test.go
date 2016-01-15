package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestInsertHost tests the insertHost method.
func TestInsertHost(t *testing.T) {
	// no dependencies necessary
	hdb := newHostDB(nil, nil, nil, nil, nil, nil)

	// invalid host should not be scanned
	hdb.insertHost(modules.HostSettings{NetAddress: "foo"})
	select {
	case <-hdb.scanPool:
		t.Error("invalid host was added to scan pool")
	case <-time.After(10 * time.Millisecond):
	}

	// valid host should be scanned
	hdb.insertHost(modules.HostSettings{NetAddress: "foo:1234"})
	select {
	case <-hdb.scanPool:
	case <-time.After(10 * time.Millisecond):
		t.Error("host was not scanned")
	}

	// duplicate host should not be scanned
	hdb.allHosts["bar:1234"] = nil
	hdb.insertHost(modules.HostSettings{NetAddress: "bar:1234"})
	select {
	case <-hdb.scanPool:
		t.Error("duplicate host was added to scan pool")
	case <-time.After(10 * time.Millisecond):
	}
}

// TestActiveHosts tests the ActiveHosts method.
func TestActiveHosts(t *testing.T) {
	// no dependencies necessary
	hdb := newHostDB(nil, nil, nil, nil, nil, nil)

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
	}
}

// TestAveragePrice tests the AveragePrice method, which also depends on the
// randomHosts function.
func TestAveragePrice(t *testing.T) {
	// no dependencies necessary
	hdb := newHostDB(nil, nil, nil, nil, nil, nil)

	// empty
	if avg := hdb.AveragePrice(); !avg.IsZero() {
		t.Error("average of empty hostdb should be zero:", avg)
	}

	// with one host
	h1 := new(hostEntry)
	h1.NetAddress = "foo"
	h1.Price = types.NewCurrency64(100)
	h1.weight = baseWeight
	hdb.insertNode(h1)
	if len(hdb.activeHosts) != 1 {
		t.Error("host was not added:", hdb.activeHosts)
	}
	if avg := hdb.AveragePrice(); avg.Cmp(h1.Price) != 0 {
		t.Error("average of one host should be that host's price:", avg)
	}

	// with two hosts
	h2 := new(hostEntry)
	h2.NetAddress = "bar"
	h2.Price = types.NewCurrency64(300)
	h2.weight = baseWeight
	hdb.insertNode(h2)
	if len(hdb.activeHosts) != 2 {
		t.Error("host was not added:", hdb.activeHosts)
	}
	if avg := hdb.AveragePrice(); avg.Cmp(types.NewCurrency64(200)) != 0 {
		t.Error("average of two hosts should be their sum/2:", avg)
	}
}
