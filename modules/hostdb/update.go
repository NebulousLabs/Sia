package hostdb

import (
	"strings"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// findHostAnnouncements returns a list of the host announcements found within
// a given block. No check is made to see that the ip address found in the
// announcement is actually a valid ip address.
func findHostAnnouncements(b types.Block) (announcements []modules.HostEntry) {
	for _, t := range b.Transactions {
		for _, data := range t.ArbitraryData {
			// the HostAnnouncement must be prefaced by the standard host announcement string
			if !strings.HasPrefix(data, modules.PrefixHostAnnouncement) {
				continue
			}

			// decode the HostAnnouncement
			var ha modules.HostAnnouncement
			encAnnouncement := []byte(strings.TrimPrefix(data, modules.PrefixHostAnnouncement))
			err := encoding.Unmarshal(encAnnouncement, &ha)
			if err != nil {
				continue
			}

			// Add the announcement to the slice being returned.
			announcements = append(announcements, modules.HostEntry{
				IPAddress: ha.IPAddress,
			})
		}
	}

	return
}

// update grabs all of the new blocks from the consensus set and searches them
// for host announcements.
func (hdb *HostDB) ReceiveConsensusUpdate(_, appliedBlocks []types.Block) {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)

	// Add hosts announced in blocks that were applied.
	for _, block := range appliedBlocks {
		for _, entry := range findHostAnnouncements(block) {
			hdb.insertHost(entry)
		}
	}

	return
}
