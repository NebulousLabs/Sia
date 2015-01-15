package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/sia/components"
)

// testHostAnnoucement has the core's host create an annoucement, and then
// checks that the host database read the annoucement.
func testHostAnnouncement(t *testing.T, c *Core) {
	// Find the existing number of hosts in the hostdb.
	prevSize := c.hostDB.Size()

	// Add test settings to the host.
	coinAddress, _, err := c.wallet.CoinAddress()
	if err != nil {
		t.Fatal(err)
	}
	hostAnnouncement := components.HostAnnouncement{
		IPAddress:          c.server.Address(),
		TotalStorage:       10 * 1000,
		MinFilesize:        64,
		MaxFilesize:        2 * 1000,
		MinDuration:        20,
		MaxDuration:        52 * 1008,
		MinChallengeWindow: 50,
		MaxChallengeWindow: 200,
		MinTolerance:       5,
		Price:              2,
		Burn:               2,
		CoinAddress:        coinAddress,
	}
	c.UpdateHost(hostAnnouncement)

	// Submit a host announcement.
	transaction, err := c.host.AnnounceHost(1500, 120)
	if err != nil {
		t.Fatal(err)
	}
	c.processTransaction(transaction) // Force the transaction to process before the block is mined.

	// Mine a block so that the host announcement is processed.
	mineSingleBlock(t, c)

	// Check that the hostdb has updated.
	if prevSize != c.hostDB.Size()-1 {
		t.Error("HostDB did not increase in size after making a host announcement and mining a block.")
	}
}
