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
		AcceptingContracts bool              `json:"acceptingcontracts"`
		Collateral         types.Currency    `json:"collateral"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		MinDuration        types.BlockHeight `json:"minduration"`
		NetAddress         NetAddress        `json:"netaddress"`
		Price              types.Currency    `json:"price"`
		TotalStorage       int64             `json:"totalstorage"`
		UnlockHash         types.UnlockHash  `json:"unlockhash"`
		WindowSize         types.BlockHeight `json:"windowsize"`
	}

	// HostRPCMetrics reports the quantity of each type of rpc call that has
	// been made to the host.
	HostRPCMetrics struct {
		ErrorCalls        uint64 `json:"errorcalls"` // Calls that resulted in an error.
		UnrecognizedCalls uint64 `json:"unrecognizedcalls"`
		DownloadCalls     uint64 `json:"downloadcalls"`
		RenewCalls        uint64 `json:"renewcalls"`
		ReviseCalls       uint64 `json:"revisecalls"`
		SettingsCalls     uint64 `json:"settingscalls"`
		UploadCalls       uint64 `json:"uploadcalls"`
	}

	// Host can take storage from disk and offer it to the network, managing things
	// such as announcements, settings, and implementing all of the RPCs of the
	// host protocol.
	Host interface {
		// AcceptingNewContracts indicates whether the host is actively
		// accepting contracts or not.
		AcceptingNewContracts() bool

		// AcceptNewContracts will cause the host to start accepting incoming
		// file contracts.
		AcceptNewContracts() error

		// Announce announces the host on the blockchain, returning an error if
		// the external ip address is unknown. After announcing, the host will
		// begin accepting new file contracts.
		Announce() error

		// AnnounceAddress announces the specified address on the blockchain.
		// After announcing, the host will begin accepting new file contracts.
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

		// Revenue returns the amount of revenue that the host has lined up,
		// the amount of revenue the host has successfully captured, and the
		// amount of revenue the host has lost.
		//
		// TODO: This function will eventually include two more numbers, one
		// representing current collateral at risk, and one representing total
		// collateral lost.
		Revenue() (unresolved, resolved, lost types.Currency)

		// RPCMetrics returns information on the types of rpc calls that have
		// been made to the host.
		RPCMetrics() HostRPCMetrics

		// RejectNewContracts will cause the host to reject all incoming
		// contracts. The host will still create storage proofs for existing
		// file contracts.
		RejectNewContracts()

		// SetConfig sets the hosting parameters of the host.
		SetSettings(HostSettings) error

		// Settings returns the host's settings.
		Settings() HostSettings

		// Close saves the state of the host and stops its listener process.
		Close() error
	}
)
