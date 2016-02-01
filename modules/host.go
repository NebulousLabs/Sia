package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// AcceptResponse defines the response that is sent to a succesful rpc.
	AcceptResponse = "accept"

	// HostDir names the directory that contains the host persistence.
	HostDir = "host"

	// MaxfileContractSetLen determines the maximum allowed size of a
	// transaction set that can be sent when trying to negotiate a file
	// contract. The transaction set will contain all of the unconfirmed
	// dependencies of the file contract, meaning that it can be quite large.
	// The transaction pool's size limit for transaction sets has been chosen
	// as a reasonable guideline for determining what is too large.
	MaxFileContractSetLen = TransactionSetSizeLimit - 1e3
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
		NetAddress         NetAddress        `json:"netaddress"`
		TotalStorage       int64             `json:"totalstorage"`
		MinDuration        types.BlockHeight `json:"minduration"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		WindowSize         types.BlockHeight `json:"windowsize"`
		Price              types.Currency    `json:"price"`
		Collateral         types.Currency    `json:"collateral"`
		UnlockHash         types.UnlockHash  `json:"unlockhash"`
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
		// Announce submits a host announcement to the blockchain, returning an
		// error if its external IP address is unknown. After announcing, the
		// host will begin accepting contracts.
		Announce() error

		// AnnounceAddress behaves like Announce, but allows the caller to
		// specify the address announced. Like Announce, this will cause the
		// host to start accepting contracts.
		AnnounceAddress(NetAddress) error

		// Capacity returns the amount of storage still available on the
		// machine. The amount can be negative if the total capacity was
		// reduced to below the active capacity.
		Capacity() int64

		// Contracts returns the number of unresolved file contracts that the
		// host is responsible for.
		Contracts() uint64

		// DeleteContract deletes a file contract. The revenue and collateral
		// on the file contract will be lost, and the data will be removed.
		DeleteContract(types.FileContractID) error

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

		// SetConfig sets the hosting parameters of the host.
		SetSettings(HostSettings) error

		// Settings returns the host's settings.
		Settings() HostSettings

		// Close saves the state of the host and stops its listener process.
		Close() error
	}
)
