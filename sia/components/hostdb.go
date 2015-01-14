package components

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	HostAnnouncementPrefix = 1
)

type HostDB interface {
	// FlagHost alerts the HostDB that a host is not behaving as expected. The
	// HostDB may decide to remove the host, or just reduce the weight, or it
	// may decide to ignore the flagging. If the flagging is ignored, an error
	// will be returned explaining why.
	FlagHost(id string) error

	// Info returns an arbitrary byte slice presumably with information about
	// the status of the hostdb. Info is not relevant to the sia package, but
	// instead toa frontend.
	Info() ([]byte, error)

	// Insert puts a host entry into the host database.
	Insert(HostEntry) error

	// Remove pulls a host entry from the host database.
	Remove(id string) error

	// RandomHost pulls a host entry at random from the database, weighted
	// according to whatever score is assigned the hosts.
	RandomHost() (HostEntry, error)

	// Size returns the number of active hosts in the hostdb.
	Size() int

	// Update gives the hostdb a set of blocks that have been applied and
	// reversed.
	Update(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) error
}

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
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
