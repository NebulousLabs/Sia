package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

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
	ht.host, err = New(ht.cs, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the address persisted.
	if ht.host.settings.NetAddress != "foo.com:234" {
		t.Error("User-set address does not seem to be persisting.")
	}
}
