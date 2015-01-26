package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	HostAnnouncementPrefix = 1
)

// the Host struct is kept in the client package because it's what the client
// uses to weigh hosts and pick them out when storing files.
type HostEntry struct {
	ID          string
	IPAddress   network.Address
	MinFilesize uint64
	MaxFilesize uint64
	MinDuration consensus.BlockHeight
	MaxDuration consensus.BlockHeight
	Window      consensus.BlockHeight
	Tolerance   uint64
	Price       consensus.Currency
	Burn        consensus.Currency
	Freeze      consensus.Currency
	CoinAddress consensus.CoinAddress
}

type HostDB interface {
	// FlagHost alerts the HostDB that a host is not behaving as expected. The
	// HostDB may decide to remove the host, or just reduce the weight, or it
	// may decide to ignore the flagging. If the flagging is ignored, an error
	// will be returned explaining why.
	FlagHost(id string) error

	// Insert puts a host entry into the host database.
	InsertHost(HostEntry) error

	// RandomHost pulls a host entry at random from the database, weighted
	// according to whatever score is assigned the hosts.
	RandomHost() (HostEntry, error)

	// Remove pulls a host entry from the host database.
	RemoveHost(id string) error
}
