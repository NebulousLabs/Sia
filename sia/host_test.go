package sia

import (
	"testing"
)

// testHostAnnoucement has the core's host create an annoucement, and then
// checks that the host database read the annoucement.
func testHostAnnouncement(t *testing.T, c *Core) {
	// Add test settings to the host.

	// Submit a host announcement.
	_, err := c.host.AnnounceHost(1500, 120)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block so that the host announcement is processed.

	// Check that the host announcement has made it into the hostdb.

	t.Fatal("more to do")
}
