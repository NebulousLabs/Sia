package contractor

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// uptimeHostDB overrides an existing hostDB so that it always returns
// IsOffline == true for a specified address.
type uptimeHostDB struct {
	hostDB
	addr modules.NetAddress
}

func (u uptimeHostDB) IsOffline(addr modules.NetAddress) bool {
	if addr == u.addr {
		return true
	}
	return u.hostDB.IsOffline(addr)
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
