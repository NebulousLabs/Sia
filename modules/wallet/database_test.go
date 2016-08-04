package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// TestDBOpen tests the wallet.openDB method.
func TestDBOpen(t *testing.T) {
	w := new(Wallet)
	err := w.openDB("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	testdir := build.TempDir(modules.WalletDir, "TestDBOpen")
	os.MkdirAll(testdir, 0700)
	err = w.openDB(filepath.Join(testdir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	w.db.View(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			bucketHistoricOutputs,
			bucketHistoricClaimStarts,
		}
		for _, b := range buckets {
			if tx.Bucket(b) == nil {
				t.Error("bucket", string(b), "does not exist")
			}
		}
		return nil
	})
	w.db.Close()
}

// TestDBHistoricHelpers tests the get/put helpers for the HistoricOutputs and
// HistoricClaimStarts buckets.
func TestDBHistoricHelpers(t *testing.T) {
	wt, err := createBlankWalletTester("TestDBHistoricOutputs")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	id := types.OutputID{1, 2, 3}
	c := types.NewCurrency64(7)
	wt.wallet.db.Update(func(tx *bolt.Tx) error {
		return dbPutHistoricOutput(tx, id, c)
	})
	wt.wallet.db.View(func(tx *bolt.Tx) error {
		c2, err := dbGetHistoricOutput(tx, id)
		if err != nil {
			t.Fatal(err)
		} else if c2.Cmp(c) != 0 {
			t.Fatal(c, c2)
		}
		return nil
	})

	soid := types.SiafundOutputID{1, 2, 3}
	c = types.NewCurrency64(7)
	wt.wallet.db.Update(func(tx *bolt.Tx) error {
		return dbPutHistoricClaimStart(tx, soid, c)
	})
	wt.wallet.db.View(func(tx *bolt.Tx) error {
		c2, err := dbGetHistoricClaimStart(tx, soid)
		if err != nil {
			t.Fatal(err)
		} else if c2.Cmp(c) != 0 {
			t.Fatal(c, c2)
		}
		return nil
	})
}
