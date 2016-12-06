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
	dbe.NetAddress = "foo.com:1234"
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
	h1.Weight = types.NewCurrency64(1)
	h1.AcceptingContracts = true
	hdb.insertNode(h1)
	if hosts := hdb.ActiveHosts(); len(hosts) != 1 {
		t.Errorf("wrong number of hosts: expected %v, got %v", 1, len(hosts))
	} else if hosts[0].NetAddress != h1.NetAddress {
		t.Errorf("ActiveHosts returned wrong host: expected %v, got %v", h1.NetAddress, hosts[0].NetAddress)
	}

	// with multiple hosts
	h2 := new(hostEntry)
	h2.NetAddress = "bar"
	h2.Weight = types.NewCurrency64(1)
	h2.AcceptingContracts = true
	hdb.insertNode(h2)
	if hosts := hdb.ActiveHosts(); len(hosts) != 2 {
		t.Errorf("wrong number of hosts: expected %v, got %v", 2, len(hosts))
	} else if hosts[0].NetAddress != h1.NetAddress && hosts[1].NetAddress != h1.NetAddress {
		t.Errorf("ActiveHosts did not contain an inserted host: %v (missing %v)", hosts, h1.NetAddress)
	} else if hosts[0].NetAddress != h2.NetAddress && hosts[1].NetAddress != h2.NetAddress {
		t.Errorf("ActiveHosts did not contain an inserted host: %v (missing %v)", hosts, h2.NetAddress)
	}
}

// TestAverageContractPrice tests the AverageContractPrice method, which also depends on the
// randomHosts method.
func TestAverageContractPrice(t *testing.T) {
	hdb := bareHostDB()

	// empty
	if avg := hdb.AverageContractPrice(); !avg.IsZero() {
		t.Error("average of empty hostdb should be zero:", avg)
	}

	// with one host
	h1 := new(hostEntry)
	h1.NetAddress = "foo"
	h1.ContractPrice = types.NewCurrency64(100)
	h1.Weight = baseWeight
	h1.AcceptingContracts = true
	hdb.insertNode(h1)
	if avg := hdb.AverageContractPrice(); avg.Cmp(h1.ContractPrice) != 0 {
		t.Error("average of one host should be that host's price:", avg)
	}

	// with two hosts
	h2 := new(hostEntry)
	h2.NetAddress = "bar"
	h2.ContractPrice = types.NewCurrency64(300)
	h2.Weight = baseWeight
	h2.AcceptingContracts = true
	hdb.insertNode(h2)
	if len(hdb.activeHosts) != 2 {
		t.Error("host was not added:", hdb.activeHosts)
	}
	if avg := hdb.AverageContractPrice(); avg.Cmp64(200) != 0 {
		t.Error("average of two hosts should be their sum/2:", avg)
	}
}
