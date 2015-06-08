package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	AcceptTermsResponse = "accept"
	HostDir             = "host"
)

// ContractTerms are the parameters agreed upon by a client and a host when
// forming a FileContract.
type ContractTerms struct {
	FileSize           uint64                // How large the file is.
	Duration           types.BlockHeight     // How long the file is to be stored.
	DurationStart      types.BlockHeight     // The block height that the storing starts (typically required to start immediately, unless it's a chained contract).
	WindowSize         types.BlockHeight     // How long the host has to submit a proof of storage.
	Price              types.Currency        // Client contribution towards payout each window
	Collateral         types.Currency        // Host contribution towards payout each window
	ValidProofOutputs  []types.SiacoinOutput // Where money goes if the storage proof is successful.
	MissedProofOutputs []types.SiacoinOutput // Where the money goes if the storage proof fails.
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

	// Announce announces the host on the blockchain.
	Announce() error

	// HostNotify will push a struct down the channel every time that an update
	// is received.
	HostNotify() <-chan struct{}

	// SetConfig sets the hosting parameters of the host.
	SetSettings(HostSettings)

	// Settings returns the host's settings.
	Settings() HostSettings

	// Info returns info about the host, including its hosting parameters, the
	// amount of storage remaining, and the number of active contracts.
	Info() HostInfo
}
