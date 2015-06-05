package modules

import (
	"github.com/NebulousLabs/Sia/types"
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
	IPAddress NetAddress
}

// HostSettings are the parameters advertised by the host. These are the
// values that the HostDB will request from the host in order to build its
// database.
type HostSettings struct {
	IPAddress    NetAddress
	TotalStorage int64 // Can go negative.
	MinFilesize  uint64
	MaxFilesize  uint64
	MinDuration  types.BlockHeight
	MaxDuration  types.BlockHeight
	WindowSize   types.BlockHeight
	Price        types.Currency
	Collateral   types.Currency
	UnlockHash   types.UnlockHash
}

// A HostDB is a database of hosts that the renter can use for figuring out who
// to upload to, and download from.
type HostDB interface {
	// ActiveHosts returns the list of hosts that are actively being selected
	// from.
	ActiveHosts() []HostSettings

	// AllHosts returns the full list of hosts known to the hostdb.
	AllHosts() []HostSettings

	// HostDBNotify will push a struct down the returned channel every time the
	// hostdb receives an update from the consensus set.
	HostDBNotify() <-chan struct{}

	// InsertHost adds a host to the database.
	InsertHost(HostSettings) error

	// RandomHosts will pull up to 'num' random hosts from the hostdb. There
	// will be no repeats, but the length of the slice returned may be less
	// than 'num', and may even be 0. The hosts returned first have the higher
	// priority.
	RandomHosts(num int) []HostSettings

	// Remove deletes the host with the input address from the database.
	RemoveHost(NetAddress) error
}
