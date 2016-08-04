package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

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
