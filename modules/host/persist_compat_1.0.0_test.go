package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestHostPersistCompat100 tests breaking changes in the host persist struct
// resulting from spelling errors. The test occurs by loading
// hostpersist_compat_1.0.0.json, a v0.6.0 host persistence file that has been
// pulled from the wild and adapted to have all non-zero values in its fields
// for the purposes of testing.
func TestHostPersistCompat100(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestHostPersistCompat100")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Close the host and then swap out the persist file for the one that is
	// being used for testing.
	ht.host.Close()
	source := filepath.Join("testdata", "v100Host.tar.gz")
	err = build.ExtractTarGz(source, filepath.Join(ht.host.persistDir))
	if err != nil {
		t.Log(filepath.Abs(source))
		t.Fatal(err)
	}
	h, err := New(ht.cs, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that, after loading the compatibility file, all of the values are
	// still correct. The file that was transplanted had no zero-value fields.
	ht.host.mu.Lock()
	if h.financialMetrics.PotentialStorageRevenue.IsZero() {
		t.Error("potential storage revenue not loaded correctly")
	}
	if h.settings.MinContractPrice.IsZero() {
		t.Error("min contract price not loaded correctly")
	}
	if h.settings.MinDownloadBandwidthPrice.IsZero() {
		t.Error("min download bandwidth price not loaded correctly")
	}
	if h.settings.MinStoragePrice.IsZero() {
		t.Error("min storage price not loaded correctly")
	}
	if h.settings.MinUploadBandwidthPrice.IsZero() {
		t.Error("min upload bandwidth price not loaded correctly")
	}
	if h.revisionNumber == 0 {
		t.Error("revision number loaded incorrectly")
	}
	if h.unlockHash == (types.UnlockHash{}) {
		t.Error("unlock hash loaded incorrectly")
	}
	ht.host.mu.Unlock()

	// Set ht.host to 'h' so that the 'ht.Close()' method will close everything
	// cleanly.
	ht.host = h
}
