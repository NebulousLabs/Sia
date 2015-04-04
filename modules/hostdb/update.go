package hostdb

import (
	"strings"

	"github.com/NebulousLabs/Sia/build"
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
// for host announcements. The threading is careful to avoid holding a lock
// while network communication is happening.
func (hdb *HostDB) update() {
	// Get the blocks that have been added since the previous update.
	_, appliedBlocks, err := hdb.state.BlocksSince(hdb.recentBlock)
	if err != nil {
		// Sanity check - err should be nil.
		if build.DEBUG {
			panic("hostdb got an error when calling hdb.state.BlocksSince")
		}
	}

	// Add hosts announced in blocks that were applied.
	for _, blockID := range appliedBlocks {
		block, exists := hdb.state.Block(blockID)
		if !exists {
			if build.DEBUG {
				panic("state is telling us a block doesn't exist that got returned by BlocksSince")
			}
			continue
		}
		for _, entry := range findHostAnnouncements(block) {
			hdb.insert(entry)
		}
		hdb.recentBlock = blockID
	}

	return
}

// threadedConsensusListen listens on a state subscription and updates every
// time that there's a new block.
func (hdb *HostDB) threadedConsensusListen() {
	sub := hdb.state.SubscribeToConsensusChanges()
	for {
		hdb.mu.Lock()
		hdb.update()
		hdb.mu.Unlock()
		<-sub
	}
}
