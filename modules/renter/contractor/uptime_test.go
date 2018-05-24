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
		lenC := c.staticContracts.Len()
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
			if len(host.ScanHistory) < 3 {
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
