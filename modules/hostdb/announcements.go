package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// findHostAnnouncements scans a block and pulls out every host announcement
// that appears in the block, returning a list of entries that correspond with
// the announcements.
func findHostAnnouncements(height consensus.BlockHeight, b consensus.Block) (entries []modules.HostEntry, err error) {
	for _, t := range b.Transactions {
		// Check the arbitrary data of the transaction to fill out the host database.
		if len(t.ArbitraryData) == 0 {
			continue
		}
		if len(t.ArbitraryData[0]) < 8 {
			continue
		}

		// TODO: switch dataIndicator
		// TODO: new announcement struct
		dataIndicator := encoding.DecUint64([]byte(t.ArbitraryData[0][0:8]))
		if dataIndicator == modules.HostAnnouncementPrefix {
			var entry modules.HostEntry
			err = encoding.Unmarshal([]byte(t.ArbitraryData[0][8:]), &entry)
			if err != nil {
				return
			}

			// Verify that the host has declared values that are relevant to our
			// interests.
			// TODO: need a way to get the freeze index
			/*
				if entry.SpendConditions.CoinAddress() != t.Outputs[entry.FreezeIndex].SpendHash {
					continue
				}

				entry.Freeze = consensus.Currency(entry.SpendConditions.TimeLock-height) * t.Outputs[entry.FreezeIndex].Value
				if entry.Freeze <= 0 {
					continue
				}
			*/

			entries = append(entries, entry)
		}
	}

	return
}
