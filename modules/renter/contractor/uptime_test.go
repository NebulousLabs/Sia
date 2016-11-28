package contractor

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// uptimeHostDB overrides an existing hostDB so that it always returns
// isOffline == true for a specified address.
type uptimeHostDB struct {
	hostDB
	addr modules.NetAddress
}

func (u uptimeHostDB) Host(addr modules.NetAddress) (modules.HostDBEntry, bool) {
	host, ok := u.hostDB.Host(addr)
	if ok && addr == u.addr {
		// fake three scans, all of which failed
		badScan := modules.HostDBScan{Timestamp: time.Now(), Success: false}
		host.ScanHistory = []modules.HostDBScan{badScan, badScan, badScan}
	}
	return host, ok
}

// TestIntegrationMonitorUptime tests that when a host goes offline, its
// contract is eventually replaced.
func TestIntegrationMonitorUptime(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	h, c, m, err := newTestingTrio("TestIntegrationMonitorUptime")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// override IsOffline to always return true for h
	c.hdb = uptimeHostDB{c.hdb, h.ExternalSettings().NetAddress}

	// create another host
	dir := build.TempDir("contractor", "TestIntegrationMonitorUptime", "Host2")
	h2, err := newTestingHost(dir, c.cs.(modules.ConsensusSet), c.tpool.(modules.TransactionPool))
	if err != nil {
		t.Fatal(err)
	}

	// form a contract with h
	c.SetAllowance(modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(100),
		Hosts:       1,
		Period:      100,
		RenewWindow: 10,
	})
	// we should have a contract
	if len(c.Contracts()) != 1 {
		t.Fatal("contract not formed")
	}

	// close the host; contractor should eventually delete contract
	h.Close()
	for i := 0; i < 100 && len(c.Contracts()) != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(c.Contracts()) != 0 {
		t.Fatal("contract was not removed")
	}

	// announce the second host
	err = h2.Announce()
	if err != nil {
		t.Fatal(err)
	}

	// mine a block, processing the announcement
	m.AddBlock()

	// wait for hostdb to scan host
	for i := 0; i < 100 && len(c.hdb.RandomHosts(2, nil)) != 2; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if len(c.hdb.RandomHosts(2, nil)) != 2 {
		t.Fatal("host did not make it into the contractor hostdb in time", c.hdb.RandomHosts(2, nil))
	}

	// mine blocks until a new contract is formed. ProcessConsensusChange will
	// trigger managedFormAllowanceContracts, which should form a new contract
	// with h2
	for i := 0; i < 100 && len(c.Contracts()) != 1; i++ {
		m.AddBlock()
		time.Sleep(100 * time.Millisecond)
	}
	if len(c.Contracts()) != 1 {
		t.Fatal("contract was not replaced")
	}
	if c.Contracts()[0].NetAddress != h2.ExternalSettings().NetAddress {
		t.Fatal("contractor formed replacement contract with wrong host")
	}
}

// TestIsOffline tests the isOffline helper function.
func TestIsOffline(t *testing.T) {
	now := time.Now()
	oldScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow * 2), Success: false}
	newBadScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: false}
	newGoodScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: true}

	tests := []struct {
		scans   []modules.HostDBScan
		offline bool
	}{
		// no data
		{nil, false},
		// not enough data
		{[]modules.HostDBScan{oldScan, newGoodScan}, false},
		// not recent enough data
		{[]modules.HostDBScan{oldScan, oldScan, oldScan}, false},
		// recent data, but at least 1 scan succeded
		{[]modules.HostDBScan{newBadScan, newGoodScan, newBadScan}, false},
		// recent data, but no scans succeded
		{[]modules.HostDBScan{newBadScan, newBadScan, newBadScan}, true},
	}
	for i, test := range tests {
		h := modules.HostDBEntry{ScanHistory: test.scans}
		if offline := isOffline(h); offline != test.offline {
			t.Errorf("IsOffline(%v) = %v, expected %v", i, offline, test.offline)
		}
	}
}
