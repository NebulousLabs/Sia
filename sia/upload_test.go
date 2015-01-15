package sia

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/sia/components"
)

func testUploadFile(t *testing.T, c *Core) {
	if !testing.Short() {
		// Check that hostDB has at least one entry.
		if c.hostDB.Size() < 1 {
			t.Fatal("Hostdb needs at least 1 host to perform testUploadFile")
		}

		// Have the renter negotiate a contract with the host in the hostDB.
		err := c.renter.RentFile(components.RentFileParameters{
			Filepath:    "sia.go",
			Nickname:    "one",
			TotalPieces: 1,
		})
		if err != nil {
			t.Error(err)
		}

		time.Sleep(10 * time.Second)

		// TODO: Check that the file has been added to the renter fileset.

		// Check that the file has been added to the host.
		if c.host.NumContracts() == 0 {
			t.Error("Host is not reporting a new contract.")
		}

		// Check that hostDB has at least one entry.
		if c.hostDB.Size() < 1 {
			t.Fatal("Hostdb got pruned while trying to make a contract?")
		}
	}
}
