package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	IPAddress    network.Address
	TotalStorage int64 // Can go negative.
	MinFilesize  uint64
	MaxFilesize  uint64
	MinDuration  consensus.BlockHeight
	MaxDuration  consensus.BlockHeight
	MinWindow    consensus.BlockHeight
	Price        consensus.Currency
	Burn         consensus.Currency
	CoinAddress  consensus.CoinAddress // Host may want to give different addresses to each client.

	SpendConditions consensus.SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}

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

		dataIndicator := encoding.DecUint64([]byte(t.ArbitraryData[0][0:8]))
		if dataIndicator == 1 {
			var ha HostAnnouncement
			err = encoding.Unmarshal([]byte(t.ArbitraryData[0][8:]), &ha)
			if err != nil {
				return
			}

			// Verify that the host has declared values that are relevant to our
			// interests.
			if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
				continue
			}
			freeze := consensus.Currency(ha.SpendConditions.TimeLock-height) * t.Outputs[ha.FreezeIndex].Value
			if freeze <= 0 {
				continue
			}

			// Add the host to the host database.
			entryID := t.OutputID(int(ha.FreezeIndex))
			entries = append(entries, modules.HostEntry{
				HostAnnouncement: ha,
				ID:               string(entryID[:]),
				Freeze:           freeze,
			})
		}
	}

	return
}
