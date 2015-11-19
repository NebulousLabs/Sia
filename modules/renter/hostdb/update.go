package hostdb

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// findHostAnnouncements returns a list of the host announcements found within
// a given block. No check is made to see that the ip address found in the
// announcement is actually a valid ip address.
func findHostAnnouncements(b types.Block) (announcements []modules.HostSettings) {
	for _, t := range b.Transactions {
		// the HostAnnouncement must be prefaced by the standard host
		// announcement string
		var prefix types.Specifier
		for _, arb := range t.ArbitraryData {
			copy(prefix[:], arb)
			if prefix != modules.PrefixHostAnnouncement {
				continue
			}

			// decode the HostAnnouncement
			var ha modules.HostAnnouncement
			err := encoding.Unmarshal(arb[types.SpecifierLen:], &ha)
			if err != nil {
				continue
			}

			// Add the announcement to the slice being returned.
			announcements = append(announcements, modules.HostSettings{
				IPAddress: ha.IPAddress,
			})
		}
	}

	return
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (hdb *HostDB) ProcessConsensusChange(cc modules.ConsensusChange) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	hdb.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
	hdb.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Add hosts announced in blocks that were applied.
	for _, block := range cc.AppliedBlocks {
		for _, host := range findHostAnnouncements(block) {
			hdb.insertHost(host)
		}
	}
}
