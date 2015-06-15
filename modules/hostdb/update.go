package hostdb

// update.go is responsible for finding new hosts and adding them to the
// database. Currently, the blockchain is the only source for finding hosts,
// and any host announcement in the blockchain is accepted with equal weight.
// The current implementation is trivially vulnerable to a sybil attack,
// whereby a host can gain favoritism by announcing itself many times using
// different addresses. We have chosen to ignore this vulnerability for the
// early stages of the network, though eventually it will be addressed by
// requiring hosts to burn coins, and weighting them according to the number of
// coins burned.

import (
	"strings"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// findHostAnnouncements returns a list of the host announcements found within
// a given block. No check is made to see that the ip address found in the
// announcement is actually a valid ip address.
func findHostAnnouncements(b types.Block) (announcements []modules.HostSettings) {
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
			announcements = append(announcements, modules.HostSettings{
				IPAddress: ha.IPAddress,
			})
		}
	}

	return
}

// ReceiveConsensusSetUpdate accepts an update from the consensus set which
// contains new blocks.
func (hdb *HostDB) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)

	// Add hosts announced in blocks that were applied.
	for _, block := range cc.AppliedBlocks {
		for _, host := range findHostAnnouncements(block) {
			hdb.insertHost(host)
		}
	}

	hdb.consensusHeight -= len(cc.RevertedBlocks)
	hdb.consensusHeight += len(cc.AppliedBlocks)
	hdb.notifySubscribers()
	return
}
