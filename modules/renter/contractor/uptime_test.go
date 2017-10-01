package contractor

import (
	"errors"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationReplaceOffline tests that when a host goes offline, its
// contract is eventually replaced.
func TestIntegrationReplaceOffline(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	h, c, m, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// form a contract with h
	c.SetAllowance(modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(250),
		Hosts:       1,
		Period:      50,
		RenewWindow: 20,
	})
	// Block until the contract is registered.
	err = build.Retry(50, 100*time.Millisecond, func() error {
		c.mu.Lock()
		lenC := len(c.contracts)
		c.mu.Unlock()
		if lenC < 1 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Log(len(c.Contracts()))
		t.Error(err)
	}

	// Take the first host offline.
	err = h.Close()
	if err != nil {
		t.Error(err)
	}
	// Block until the host is seen as offline.
	hosts := c.hdb.AllHosts()
	err = build.Retry(250, 250*time.Millisecond, func() error {
		hosts = c.hdb.AllHosts()
		if len(hosts) != 1 {
			return errors.New("only expecting one host")
		}
		sh := hosts[0].ScanHistory
		if sh[len(sh)-1].Success {
			return errors.New("host is reporting as online")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// create another host
	dir := build.TempDir("contractor", t.Name(), "Host2")
	h2, err := newTestingHost(dir, c.cs.(modules.ConsensusSet), c.tpool.(modules.TransactionPool))
	if err != nil {
		t.Fatal(err)
	}
	// Announce the second host.
	err = h2.Announce()
	if err != nil {
		t.Fatal(err)
	}
	// Mine a block to get the announcement on-chain.
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Wait for a scan of the host to complete.
	err = build.Retry(250, 250*time.Millisecond, func() error {
		hosts = c.hdb.AllHosts()
		if len(hosts) < 2 {
			return errors.New("waiting for at least two hosts to show up")
		}
		for _, host := range hosts {
			if len(host.ScanHistory) < 2 {
				return errors.New("waiting for the hosts to have been scanned")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine 3 blocks to trigger an allowance refresh, which should cause the
	// second, online host to be picked up. Three are mined because mining just
	// one was causing NDFs.
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	var numContracts int
	err = build.Retry(250, 250*time.Millisecond, func() error {
		numContracts = len(c.Contracts())
		if numContracts < 2 {
			return errors.New("still waiting to form the second contract")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err, numContracts)
	}
}

// mapHostDB is a hostDB that implements the Host method via a simple map.
type mapHostDB struct {
	stubHostDB
	hosts map[string]modules.HostDBEntry
}

func (m mapHostDB) Host(spk types.SiaPublicKey) (modules.HostDBEntry, bool) {
	h, e := m.hosts[string(spk.Key)]
	return h, e
}

// TestIsOffline tests the IsOffline method.
func TestIsOffline(t *testing.T) {
	now := time.Now()
	oldBadScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow * 2), Success: false}
	oldGoodScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow * 2), Success: true}
	newBadScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: false}
	newGoodScan := modules.HostDBScan{Timestamp: now.Add(-uptimeWindow / 2), Success: true}
	currentBadScan := modules.HostDBScan{Timestamp: now, Success: false}
	currentGoodScan := modules.HostDBScan{Timestamp: now, Success: true}

	tests := []struct {
		scans   []modules.HostDBScan
		offline bool
	}{
		// no data
		{nil, true},
		// not enough data
		{[]modules.HostDBScan{oldBadScan, newGoodScan}, false},
		// data covers small range
		{[]modules.HostDBScan{oldBadScan, oldBadScan, oldBadScan}, true},
		// data covers large range, but at least 1 scan succeeded
		{[]modules.HostDBScan{oldBadScan, newGoodScan, currentBadScan}, true},
		// data covers large range, no scans succeeded
		{[]modules.HostDBScan{oldBadScan, newBadScan, currentBadScan}, true},
		// old scan was good, recent scans are bad.
		{[]modules.HostDBScan{oldGoodScan, newBadScan, newBadScan, currentBadScan}, true},
		// recent scan was good, with many recent bad scans.
		{[]modules.HostDBScan{oldBadScan, newGoodScan, newBadScan, currentBadScan, currentBadScan}, true},
		// recent scan was good, old scans were bad.
		{[]modules.HostDBScan{oldBadScan, newBadScan, currentBadScan, currentGoodScan}, false},
	}
	for i, test := range tests {
		// construct a contractor with a hostdb containing the scans
		c := &Contractor{
			contracts: map[types.FileContractID]modules.RenterContract{
				{1}: {HostPublicKey: types.SiaPublicKey{Key: []byte("foo")}},
			},
			hdb: mapHostDB{
				hosts: map[string]modules.HostDBEntry{
					"foo": {ScanHistory: test.scans},
				},
			},
		}
		if offline := c.IsOffline(types.FileContractID{1}); offline != test.offline {
			t.Errorf("IsOffline(%v) = %v, expected %v", i, offline, test.offline)
		}
	}
	c := &Contractor{
		contracts: map[types.FileContractID]modules.RenterContract{
			{1}: {HostPublicKey: types.SiaPublicKey{Key: []byte("foo")}},
		},
	}
	// should return true for an unknown contract id
	if !c.IsOffline(types.FileContractID{4}) {
		t.Fatal("IsOffline returned false for a nonexistent contract id")
	}
}
