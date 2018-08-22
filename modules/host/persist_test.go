package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestHostContractCountPersistence checks that the host persists its contract
// counts correctly
func TestHostContractCountPersistence(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// add a storage obligation, which should increment contract count
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())

	// should have 1 contract now
	if ht.host.financialMetrics.ContractCount != 1 {
		t.Fatal("expected one contract, got", ht.host.financialMetrics.ContractCount)
	}

	// reload the host
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = New(ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// contract count should still be 1
	if ht.host.financialMetrics.ContractCount != 1 {
		t.Fatal("expected one contract, got", ht.host.financialMetrics.ContractCount)
	}
}

// TestHostAddressPersistence checks that the host persists any updates to the
// address upon restart.
func TestHostAddressPersistence(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Set the address of the host.
	settings := ht.host.InternalSettings()
	settings.NetAddress = "foo.com:234"
	err = ht.host.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Reboot the host.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = New(ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the address persisted.
	if ht.host.settings.NetAddress != "foo.com:234" {
		t.Error("User-set address does not seem to be persisting.")
	}
}
