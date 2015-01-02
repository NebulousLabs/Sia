package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	ID string

	IPAddress          network.Address
	TotalStorage       int64 // Can go negative.
	MinFilesize        uint64
	MaxFilesize        uint64
	MinDuration        consensus.BlockHeight
	MaxDuration        consensus.BlockHeight
	MinChallengeWindow consensus.BlockHeight
	MaxChallengeWindow consensus.BlockHeight
	MinTolerance       uint64
	Price              consensus.Currency
	Burn               consensus.Currency
	CoinAddress        consensus.CoinAddress // Host may want to give different addresses to each client.

	SpendConditions consensus.SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}
