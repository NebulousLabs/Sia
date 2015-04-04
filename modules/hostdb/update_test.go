package hostdb

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestFindHostAnnouncements checks that host announcements are being found and
// then correctly added to the host database.
func TestFindHostAnnouncements(t *testing.T) {
	hdbt := CreateHostDBTester("TestFindHostAnnouncements", t)

	// Call update and check the size of the hostdb, size should be 0.
	if hdbt.NumHosts() != 0 {
		t.Error("host found after initialization")
	}

	// Submit a transaction to the blockchain with random arbitrary data, check
	// that it's not interpreted as a host announcement.
	noAnnouncementTxn := types.Transaction{
		ArbitraryData: []string{"bad data"},
	}
	hdbt.MineAndSubmitCurrentBlock([]types.Transaction{noAnnouncementTxn})
	if len(hdbt.allHosts) != 0 {
		t.Error("expecting 0 hosts in allHosts, got:", len(hdbt.allHosts))
	}

	// Submit a transaction to the blockchain that says it's a host
	// announcement, but doesn't decode into one, and check that it's not
	// interpreted as one.
	dirtyAnnouncementTxn := types.Transaction{
		ArbitraryData: []string{modules.PrefixHostAnnouncement},
	}
	hdbt.MineAndSubmitCurrentBlock([]types.Transaction{dirtyAnnouncementTxn})
	hdbt.mu.Lock()
	if len(hdbt.allHosts) != 0 {
		t.Error("expecting 0 hosts in allHosts, got:", len(hdbt.allHosts))
	}
	hdbt.mu.Unlock()

	// Submit a host announcement to the blockchain for a host that won't
	// respond.
	falseAnnouncement := string(encoding.Marshal(modules.HostAnnouncement{
		IPAddress: modules.NetAddress(":4500"),
	}))
	falseAnnouncementTxn := types.Transaction{
		ArbitraryData: []string{modules.PrefixHostAnnouncement + falseAnnouncement},
	}
	hdbt.MineAndSubmitCurrentBlock([]types.Transaction{falseAnnouncementTxn})

	// Update the host db and check that the announcement made it to the
	// inactive set of hosts.
	for {
		hdbt.mu.RLock()
		if len(hdbt.allHosts) == 1 {
			break
		}
		hdbt.mu.RUnlock()
		time.Sleep(time.Millisecond)
	}
}
