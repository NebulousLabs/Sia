package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	// Denotes a host announcement in the Arbitrary Data section.
	PrefixHostAnnouncement = "HostAnnouncement"
)

// HostAnnouncements are stored in the Arbitrary Data section of transactions
// on the blockchain. They announce the willingness of a node to host files.
// Renters can contact the host privately to obtain more detailed hosting
// parameters (see HostSettings). To mitigate Sybil attacks, HostAnnouncements
// are paired with a volume of 'frozen' coins. The FreezeIndex indicates which
// output in the transaction contains the frozen coins, and the
// SpendConditions indicate the number of blocks the coins are frozen for.
type HostAnnouncement struct {
	IPAddress       network.Address
	FreezeIndex     uint64 // the index of the output that froze coins
	SpendConditions consensus.UnlockConditions
}

// HostSettings are the parameters advertised by the host. These are the
// values that the HostDB will request from the host in order to build its
// database.
type HostSettings struct {
	TotalStorage int64 // Can go negative.
	MinFilesize  uint64
	MaxFilesize  uint64
	MinDuration  consensus.BlockHeight
	MaxDuration  consensus.BlockHeight
	MinWindow    consensus.BlockHeight
	Price        consensus.Currency
	Collateral   consensus.Currency
	CoinAddress  consensus.UnlockHash
}

// A HostEntry is an entry in the HostDB. It contains the HostSettings, as
// well as the IP address where the host can be found, and the value of the
// coins frozen in the host's announcement transaction.
type HostEntry struct {
	HostSettings
	IPAddress network.Address
	Freeze    consensus.Currency // actual units are Currency * BlockHeight: "CoinBlocks"
}

type HostDB interface {
	// FlagHost alerts the HostDB that a host is not behaving as expected. The
	// HostDB may decide to remove the host, or just reduce the weight, or it
	// may decide to ignore the flagging. If the flagging is ignored, an error
	// will be returned explaining why.
	FlagHost(network.Address) error

	// Insert adds a host to the database.
	Insert(HostEntry) error

	// RandomHost pulls a host entry at random from the database, weighted
	// according to whatever score is assigned the hosts.
	RandomHost() (HostEntry, error)

	// Remove deletes the host with the given address from the database.
	Remove(network.Address) error
}
