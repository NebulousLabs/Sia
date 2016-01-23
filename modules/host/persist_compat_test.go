package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// buildCompat04Host creates a compatibility persist file for the host, but
// does not save it. When the host closes, it saves, which means the
// compatibility struct must be created before closing but saved after closing.
func (ht *hostTester) buildCompat04Host() compat04Host {
	c04h := compat04Host{
		SpaceRemaining: ht.host.spaceRemaining,
		FileCounter:    int(ht.host.fileCounter),
		Profit:         ht.host.revenue,
		HostSettings:   ht.host.settings,
		SecretKey:      ht.host.secretKey,
		PublicKey:      ht.host.publicKey,
	}
	for _, obligation := range ht.host.obligationsByID {
		compatObligation := compat04Obligation{
			ID:           obligation.ID,
			FileContract: obligation.OriginTransaction.FileContracts[0],
			Path:         obligation.Path,
		}
		c04h.Obligations = append(c04h.Obligations, compatObligation)
	}
	return c04h
}

// TestPersistCompat04 checks that the compatibility loader for version 0.4.x
// obligations is functioning.
func TestPersistCompat04(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestPersistCompat04")
	if err != nil {
		t.Fatal(err)
	}

	// Upload a file and then save the host as a compatibility host.
	_, err = ht.uploadFile("TestPersistCompat04 - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}
	// Mine a block so that the file contract ends up in the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	c04h := ht.buildCompat04Host()
	if err != nil {
		t.Fatal(err)
	}
	// Save the compatibility file, replacing the usual file with an old
	// format.
	err = persist.SaveFile(compat04Metadata, c04h, filepath.Join(ht.host.persistDir, settingsFile))
	if err != nil {
		t.Fatal(err)
	}

	// Re-open the host, which will be loading from the compatibility file.
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(rebootHost.obligationsByID) != 1 {
		t.Fatal(len(rebootHost.obligationsByID))
	}

	// Mine until the storage proof goes through, and the obligation gets
	// cleared.
	for i := types.BlockHeight(0); i <= testUploadDuration+confirmationRequirement+defaultWindowSize; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(rebootHost.obligationsByID) != 0 {
		t.Error("obligations did not clear")
	}
	if rebootHost.revenue.IsZero() {
		t.Error("host is reporting no revenue after doing a compatibility storage proof")
	}
}
