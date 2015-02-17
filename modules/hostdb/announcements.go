package hostdb

import (
	"strings"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// findHostAnnouncements returns a list of the host announcements found within
// a given block.
func findHostAnnouncements(height consensus.BlockHeight, b consensus.Block) (announcements []modules.HostEntry) {
	for _, t := range b.Transactions {
		for _, data := range t.ArbitraryData {
			// the HostAnnouncement must be prefaced by the standard host announcement string
			if !strings.HasPrefix(data, modules.HostAnnouncementPrefix) {
				continue
			}

			// decode the HostAnnouncement
			var ha modules.HostAnnouncement
			encAnnouncement := []byte(strings.TrimPrefix(data, modules.HostAnnouncementPrefix))
			err := encoding.Unmarshal(encAnnouncement, &ha)
			if err != nil {
				continue
			}

			// check that spend conditions are valid
			if ha.SpendConditions.UnlockHash() != t.SiacoinOutputs[ha.FreezeIndex].UnlockHash {
				continue
			}

			// calculate freeze and check for sane value
			timelockCost := consensus.NewCurrency64(uint64(ha.SpendConditions.Timelock - height))
			freeze := timelockCost.Mul(t.SiacoinOutputs[ha.FreezeIndex].Value)
			if freeze.Sign() <= 0 {
				continue
			}

			// At this point, the HostSettings are unknown. Before inserting
			// the host, the HostDB will call threadedInsertFromAnnouncement
			// to fill out the HostSettings.
			announcements = append(announcements, modules.HostEntry{
				IPAddress: ha.IPAddress,
				Freeze:    freeze,
			})
		}
	}

	return
}

// threadedInsertFromAnnouncement requests a host's hosting parameters, and inserts
// the resulting HostEntry into the database.
func (hdb *HostDB) threadedInsertFromAnnouncement(entry modules.HostEntry) {
	err := entry.IPAddress.RPC("HostSettings", nil, &entry.HostSettings)
	if err != nil {
		return
	}
	hdb.Insert(entry)
}
