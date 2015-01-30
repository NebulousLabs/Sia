package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	HostAnnouncementPrefix = 1
)

// A HostEntry contains information about a host on the network. The HostDB
// uses this information to select an optimal host.
type HostEntry struct {
	IPAddress    network.Address
	TotalStorage int64 // Can go negative.
	MinFilesize  uint64
	MaxFilesize  uint64
	MinDuration  consensus.BlockHeight
	MaxDuration  consensus.BlockHeight
	MinWindow    consensus.BlockHeight
	Price        consensus.Currency
	Burn         consensus.Currency
	Freeze       consensus.Currency
	CoinAddress  consensus.CoinAddress // Host may want to give different addresses to each client.

	SpendConditions consensus.SpendConditions
}

type HostDB interface {
	// FlagHost alerts the HostDB that a host is not behaving as expected. The
	// HostDB may decide to remove the host, or just reduce the weight, or it
	// may decide to ignore the flagging. If the flagging is ignored, an error
	// will be returned explaining why.
	FlagHost(network.Address) error

	// Insert adds a host to the database.
	InsertHost(HostEntry) error

	// RandomHost pulls a host entry at random from the database, weighted
	// according to whatever score is assigned the hosts.
	RandomHost() (HostEntry, error)

	// Remove deletes the host with the given address from the database.
	RemoveHost(network.Address) error
}
