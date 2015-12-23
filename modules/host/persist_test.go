package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestEarlySaving checks that the early host is correctly saving values to
// disk.
func TestEarlySaving(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestEarlySaving")
	if err != nil {
		t.Fatal(err)
	}

	// Store a few of the important fields.
	var oldSK crypto.SecretKey
	copy(oldSK[:], ht.host.secretKey[:])
	oldFileCounter := ht.host.fileCounter
	oldSpaceRemaining := ht.host.spaceRemaining
	oldProfit := ht.host.profit

	// Corrupt the fields.
	ht.host.secretKey[0]++
	ht.host.fileCounter += 7e6
	ht.host.spaceRemaining += 25e9
	ht.host.profit = ht.host.profit.Add(types.NewCurrency64(91e3))

	// Load the host and see that the fields are reset correctly.
	err = ht.host.load()
	if err != nil {
		t.Fatal(err)
	}
	if ht.host.secretKey != oldSK {
		t.Error("secret key not loaded correctly")
	}
	if ht.host.fileCounter != oldFileCounter {
		t.Error("file counter not loaded correctly")
	}
	if ht.host.spaceRemaining != oldSpaceRemaining {
		t.Error("space remaining not loaded correctly")
	}
	if ht.host.profit.Cmp(oldProfit) != 0 {
		t.Error("profit not loaded correctly")
	}
}

// TestIntegrationValuePersistence verifies that changes made to the host persist between
// loads.
func TestIntegrationValuePersistence(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestIntegrationValuePersistence")
	if err != nil {
		t.Fatal(err)
	}

	// Change one of the features of the host persistence and save.
	ht.host.fileCounter += 1500
	oldFileCounter := ht.host.fileCounter
	err = ht.host.save()
	if err != nil {
		t.Fatal(err)
	}

	// Close the current host and create a new host pointing to the same file.
	ht.host.Close()
	newHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	// Check that the adjusted value has persisted.
	if newHost.fileCounter != oldFileCounter {
		t.Fatal(err)
	}
}
