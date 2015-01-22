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
	hostAnnouncement := components.HostAnnouncement{
		TotalStorage: 10 * 1000,
		MaxFilesize:  2 * 1000,
		Price:        2,
		Burn:         2,
	}
	c.UpdateHost(hostAnnouncement)

	// Submit a host announcement.
	_, err := c.host.AnnounceHost(1500, 120)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block so that the host announcement is processed.
	// mineSingleBlock(t, c)

	// Check that the hostdb has updated.
	if prevSize != c.hostDB.Size()-1 {
		t.Error("HostDB did not increase in size after making a host announcement and mining a block.")
	}
}
