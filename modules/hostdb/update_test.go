package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// TestFindHostAnnouncements checks that host announcements are being found and
// then correctly added to the host database.
func TestFindHostAnnouncements(t *testing.T) {
	hdbt := CreateHostDBTester(t)

	// Call update and check the size of the hostdb, size should be 0.
	hdbt.update()
	if hdbt.NumHosts() != 0 {
		t.Error("host found after initialization")
	}

	// Submit a host announcement to the blockchain for a host that won't
	// respond.
	falseAnnouncement := string(encoding.Marshal(modules.HostAnnouncement{
		IPAddress: network.Address(":4500"),
	}))
	falseAnnouncementTxn := consensus.Transaction{
		ArbitraryData: []string{modules.PrefixHostAnnouncement + falseAnnouncement},
	}
	falseAnnouncementBlock := hdbt.MineCurrentBlock([]consensus.Transaction{falseAnnouncementTxn})
	err := hdbt.AcceptBlock(falseAnnouncementBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Update the host db and check that the announcement made it to the
	// inactive set of hosts.
	hdbt.update()
	if len(hdbt.allHosts) != 1 {
		t.Error("expecting 1 host in allHosts, got:", len(hdbt.allHosts))
	}
}
