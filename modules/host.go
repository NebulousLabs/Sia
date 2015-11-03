package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	AcceptResponse = "accept"
	HostDir        = "host"
)

// RPC identifiers
var (
	// Each identifier has a version number at the end, which will be
	// incremented whenever the protocol changes.
	RPCSettings = types.Specifier{'S', 'e', 't', 't', 'i', 'n', 'g', 's', 0}
	RPCUpload   = types.Specifier{'U', 'p', 'l', 'o', 'a', 'd', 0}
	RPCRevise   = types.Specifier{'R', 'e', 'v', 'i', 's', 'e', 0}
	RPCDownload = types.Specifier{'D', 'o', 'w', 'n', 'l', 'o', 'a', 'd', 0}
)

var (
	PrefixHostAnnouncement = types.Specifier{'H', 'o', 's', 't', 'A', 'n', 'n', 'o', 'u', 'n', 'c', 'e', 'm', 'e', 'n', 't'}
)

// HostAnnouncements are stored in the Arbitrary Data section of transactions
// on the blockchain. They announce the willingness of a node to host files.
// Renters can contact the host privately to obtain more detailed hosting
// parameters (see HostSettings).
type HostAnnouncement struct {
	IPAddress NetAddress
}

// HostSettings are the parameters advertised by the host. These are the
// values that the renter will request from the host in order to build its
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

// A DownloadRequest is used to retrieve a particular segment of a file from a
// host.
type DownloadRequest struct {
	Offset uint64
	Length uint64
}

// HostInfo contains HostSettings and details pertinent to the host's understanding
// of their offered services
type HostInfo struct {
	HostSettings

	StorageRemaining int64
	NumContracts     int
	Profit           types.Currency
	PotentialProfit  types.Currency

	Competition types.Currency
}

type Host interface {
	// Address returns the host's network address
	Address() NetAddress

	// Announce announces the host on the blockchain, returning an error if the
	// host cannot reach itself or if the external ip address is unknown.
	Announce() error

	// ForceAnnounce announces the specified address on the blockchain,
	// regardless of connectivity.
	ForceAnnounce(NetAddress) error

	// SetConfig sets the hosting parameters of the host.
	SetSettings(HostSettings)

	// Settings returns the host's settings.
	Settings() HostSettings

	// Info returns info about the host, including its hosting parameters, the
	// amount of storage remaining, and the number of active contracts.
	Info() HostInfo

	// Close saves the state of the host and stops its listener process.
	Close() error
}
