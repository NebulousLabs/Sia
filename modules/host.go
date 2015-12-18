package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// AcceptResponse defines the response that is sent to a succesful rpc.
	AcceptResponse = "accept"

	// HostDir names the directory that contains the host persistence.
	HostDir = "host"
)

var (
	// RPCSettings is the specifier for requesting settings from the host.
	RPCSettings = types.Specifier{'S', 'e', 't', 't', 'i', 'n', 'g', 's'}

	// RPCUpload is the specifier for initiating an upload with the host.
	RPCUpload = types.Specifier{'U', 'p', 'l', 'o', 'a', 'd'}

	// RPCRenew is the specifier to renewing an existing contract.
	RPCRenew = types.Specifier{'R', 'e', 'n', 'e', 'w'}

	// RPCRevise is the specifier for revising an existing file contract.
	RPCRevise = types.Specifier{'R', 'e', 'v', 'i', 's', 'e'}

	// RPCDownload is the specifier for downloading a file from a host.
	RPCDownload = types.Specifier{'D', 'o', 'w', 'n', 'l', 'o', 'a', 'd'}

	// PrefixHostAnnouncement is used to indicate that a transaction's
	// Arbitrary Data field contains a host announcement. The encoded
	// announcement will follow this prefix.
	PrefixHostAnnouncement = types.Specifier{'H', 'o', 's', 't', 'A', 'n', 'n', 'o', 'u', 'n', 'c', 'e', 'm', 'e', 'n', 't'}
)

type (
	// A DownloadRequest is used to retrieve a particular segment of a file from a
	// host.
	DownloadRequest struct {
		Offset uint64
		Length uint64
	}

	// HostAnnouncement declares a nodes intent to be a host, providing a net
	// address that can be used to contact the host.
	HostAnnouncement struct {
		IPAddress NetAddress
	}

	// HostSettings are the parameters advertised by the host. These are the
	// values that the renter will request from the host in order to build its
	// database.
	HostSettings struct {
		IPAddress    NetAddress
		TotalStorage int64  // Can go negative.
		MinFilesize  uint64 // DEPRECATED.
		MaxFilesize  uint64 // DEPRECATED.
		MinDuration  types.BlockHeight
		MaxDuration  types.BlockHeight
		WindowSize   types.BlockHeight
		Price        types.Currency
		Collateral   types.Currency
		UnlockHash   types.UnlockHash
	}

	// Host can take storage from disk and offer it to the network, managing things
	// such as announcements, settings, and implementing all of the RPCs of the
	// host protocol.
	Host interface {
		// Announce announces the host on the blockchain, returning an error if the
		// external ip address is unknown.
		Announce() error

		// AnnounceAddress announces the specified address on the blockchain.
		AnnounceAddress(NetAddress) error

		// Capacity returns the amount of storage still available on the
		// machine. The amount can be negative if the total capacity was
		// reduced to below the active capacity.
		Capacity() int64

		// Contracts returns the number of unresolved file contracts that the
		// host is responsible for.
		Contracts() uint64

		// NetAddress returns the host's network address
		NetAddress() NetAddress

		// Revenue returns the amount of revenue that the host has lined up, as
		// well as the amount of revenue that the host has successfully
		// captured.
		Revenue() (unresolved, resolved types.Currency)

		// SetConfig sets the hosting parameters of the host.
		SetSettings(HostSettings)

		// Settings returns the host's settings.
		Settings() HostSettings

		// Close saves the state of the host and stops its listener process.
		Close() error
	}
)
