package hostdb

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// findHostAnnouncements returns a list of the host announcements found within
// a given block. No check is made to see that the ip address found in the
// announcement is actually a valid ip address.
func findHostAnnouncements(b types.Block) (announcements []modules.HostDBEntry) {
	for _, t := range b.Transactions {
		// the HostAnnouncement must be prefaced by the standard host
		// announcement string
		for _, arb := range t.ArbitraryData {
			addr, pubKey, err := modules.DecodeAnnouncement(arb)
			if err != nil {
				continue
			}

			// Add the announcement to the slice being returned.
			var host modules.HostDBEntry
			host.NetAddress = addr
			host.PublicKey = pubKey
			announcements = append(announcements, host)
		}
	}
	return
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change in the blockchain. Updates will always be called in order.
func (hdb *HostDB) ProcessConsensusChange(cc modules.ConsensusChange) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// Update the hostdb's understanding of the block height.
	for _, block := range cc.RevertedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if hdb.blockHeight > 0 || block.ID() != types.GenesisID {
			hdb.blockHeight--
		} else if hdb.blockHeight != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			hdb.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", hdb.blockHeight)
			hdb.blockHeight = 0
		}
	}
	for _, block := range cc.AppliedBlocks {
		// Only doing the block check if the height is above zero saves hashing
		// and saves a nontrivial amount of time during IBD.
		if hdb.blockHeight > 0 || block.ID() != types.GenesisID {
			hdb.blockHeight++
		} else if hdb.blockHeight != 0 {
			// Sanity check - if the current block is the genesis block, the
			// miner height should be set to zero.
			hdb.log.Critical("Miner has detected a genesis block, but the height of the miner is set to ", hdb.blockHeight)
			hdb.blockHeight = 0
		}
	}

	// Add hosts announced in blocks that were applied.
	for _, block := range cc.AppliedBlocks {
		for _, host := range findHostAnnouncements(block) {
			hdb.log.Debugln("Found a host in a host announcement:", host.NetAddress, host.PublicKey)
			hdb.insertHost(host)
		}
	}

	hdb.lastChange = cc.ID
	err := hdb.save()
	if err != nil {
		hdb.log.Println("Error saving hostdb:", err)
	}
}
