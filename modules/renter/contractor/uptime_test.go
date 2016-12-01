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
		// fake three scans over the past uptimeWindow, all of which failed
		badScan1 := modules.HostDBScan{Timestamp: time.Now().Add(-uptimeWindow * 2), Success: false}
		badScan2 := modules.HostDBScan{Timestamp: time.Now().Add(-uptimeWindow), Success: false}
		badScan3 := modules.HostDBScan{Timestamp: time.Now(), Success: false}
		host.ScanHistory = []modules.HostDBScan{badScan1, badScan2, badScan3}
	}
	return host, ok
}

// TestIntegrationReplaceOffline tests that when a host goes offline, its
// contract is eventually replaced.
func TestIntegrationReplaceOffline(t *testing.T) {
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
	// we should have a contract, but it will be marked as offline due to the
	// hocked hostDB
	if len(c.contracts) != 1 {
		t.Fatal("contract not formed")
	}
	if len(c.onlineContracts()) != 0 {
		t.Fatal("contract should not be reported as online")
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

	// mine a block and wait for a new contract is formed. ProcessConsensusChange will
	// trigger managedFormAllowanceContracts, which should form a new contract
	// with h2
	m.AddBlock()
	for i := 0; i < 100 && len(c.Contracts()) != 1; i++ {
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
	oldBadScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow * 2), Success: false}
	newBadScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: false}
	newGoodScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: true}
	currentBadScan := modules.HostDBScan{Timestamp: now, Success: false}

	tests := []struct {
		scans   []modules.HostDBScan
		offline bool
	}{
		// no data
		{nil, false},
		// not enough data
		{[]modules.HostDBScan{oldBadScan, newGoodScan}, false},
		// data covers small range
		{[]modules.HostDBScan{oldBadScan, oldBadScan, oldBadScan}, false},
		// data covers large range, but at least 1 scan succeded
		{[]modules.HostDBScan{oldBadScan, newGoodScan, currentBadScan}, false},
		// data covers large range, no scans succeded
		{[]modules.HostDBScan{oldBadScan, newBadScan, currentBadScan}, true},
	}
	for i, test := range tests {
		h := modules.HostDBEntry{ScanHistory: test.scans}
		if offline := isOffline(h); offline != test.offline {
			t.Errorf("IsOffline(%v) = %v, expected %v", i, offline, test.offline)
		}
	}
}
