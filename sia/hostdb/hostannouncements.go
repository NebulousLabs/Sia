package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/sia/components"
)

// findHostAnnouncements scans a block and pulls out every host announcement
// that appears in the block, returning a list of entries that correspond with
// the announcements.
func findHostAnnouncements(height consensus.BlockHeight, b consensus.Block) (entries []components.HostEntry, err error) {
	for _, t := range b.Transactions {
		// Check the arbitrary data of the transaction to fill out the host database.
		if len(t.ArbitraryData) == 0 {
			continue
		}
		if len(t.ArbitraryData[0]) < 8 {
			continue
		}

		dataIndicator := encoding.DecUint64([]byte(t.ArbitraryData[0][0:8]))
		if dataIndicator == 1 {
			var ha components.HostAnnouncement
			err = encoding.Unmarshal([]byte(t.ArbitraryData[0][8:]), &ha)
			if err != nil {
				return
			}

			// Verify that the host has declared values that are relevant to our
			// interests.
			if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
				continue
			}
			if ha.MinChallengeWindow > 100 {
				continue
			}
			if ha.MinTolerance > 10 {
				continue
			}
			freeze := consensus.Currency(ha.SpendConditions.TimeLock-height) * t.Outputs[ha.FreezeIndex].Value
			if freeze <= 0 {
				continue
			}

			// Add the host to the host database.
			entryID := t.OutputID(int(ha.FreezeIndex))
			entries = append(entries, components.HostEntry{
				ID:          string(entryID[:]),
				IPAddress:   ha.IPAddress,
				MinFilesize: ha.MinFilesize,
				MaxFilesize: ha.MaxFilesize,
				MinDuration: ha.MinDuration,
				MaxDuration: ha.MaxDuration,
				Window:      ha.MinChallengeWindow,
				Tolerance:   ha.MinTolerance,
				Price:       ha.Price,
				Burn:        ha.Burn,
				Freeze:      freeze,
				CoinAddress: ha.CoinAddress,
			})
		}
	}

	return
}
