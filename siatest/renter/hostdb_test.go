package renter

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
)

// TestInitialScanComplete tests if the initialScanComplete field is set
// correctly.
func TestInitialScanComplete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Get a directory for testing.
	testDir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	testDir = filepath.Join(testDir, t.Name())

	// Create a group. The renter should block the scanning thread using a
	// dependency.
	deps := &dependencyBlockScan{}
	renterTemplate := node.Renter(filepath.Join(testDir, "renter"))
	renterTemplate.SkipSetAllowance = true
	renterTemplate.SkipHostDiscovery = true
	renterTemplate.HostDBDeps = deps

	tg, err := siatest.NewGroup(renterTemplate, node.Host(filepath.Join(testDir, "host")),
		siatest.Miner(filepath.Join(testDir, "miner")))
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		deps.Scan()
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// The renter should have 1 offline host in its database and
	// initialScanComplete should be false.
	renter := tg.Renters()[0]
	hdag, err := renter.HostDbAllGet()
	if err != nil {
		t.Fatal(err)
	}
	hdg, err := renter.HostDbGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(hdag.Hosts) != 1 {
		t.Fatalf("HostDB should have 1 host but had %v", len(hdag.Hosts))
	}
	if hdag.Hosts[0].ScanHistory.Len() > 0 {
		t.Fatalf("Host should have 0 scans but had %v", hdag.Hosts[0].ScanHistory.Len())
	}
	if hdg.InitialScanComplete {
		t.Fatal("Initial scan is complete even though it shouldn't")
	}

	deps.Scan()
	err = build.Retry(600, 100*time.Millisecond, func() error {
		hdag, err := renter.HostDbAllGet()
		if err != nil {
			t.Fatal(err)
		}
		hdg, err := renter.HostDbGet()
		if err != nil {
			t.Fatal(err)
		}
		if !hdg.InitialScanComplete {
			return fmt.Errorf("Initial scan is not complete even though it should be")
		}
		if len(hdag.Hosts) != 1 {
			return fmt.Errorf("HostDB should have 1 host but had %v", len(hdag.Hosts))
		}
		if hdag.Hosts[0].ScanHistory.Len() == 0 {
			return fmt.Errorf("Host should have >0 scans but had %v", hdag.Hosts[0].ScanHistory.Len())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
