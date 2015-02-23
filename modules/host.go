package modules

import (
	"net"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	AcceptContractResponse = "accept"
)

// ContractTerms are the parameters agreed upon by a client and a host when
// forming a FileContract.
type ContractTerms struct {
	FileSize           uint64                    // How large the file is.
	Duration           consensus.BlockHeight     // How long the file is to be stored.
	DurationStart      consensus.BlockHeight     // The block height that the storing starts (typically required to start immediately, unless it's a chained contract).
	WindowSize         consensus.BlockHeight     // How long the host has to submit a proof of storage.
	Price              consensus.Currency        // Client contribution towards payout each window
	Collateral         consensus.Currency        // Host contribution towards payout each window
	ValidProofOutputs  []consensus.SiacoinOutput // Where money goes if the storage proof is successful.
	MissedProofOutputs []consensus.SiacoinOutput // Where the money goes if the storage proof fails.
}

type HostInfo struct {
	HostSettings

	StorageRemaining int64
	NumContracts     int
}

type Host interface {
	// Announce announces the host on the blockchain.
	Announce(addr network.Address) error

	// NegotiateContract is a strict function that enables a client to
	// communicate with the host to propose a contract.
	//
	// TODO: enhance this documentataion. For now, see the host package for a
	// reference implementation.
	NegotiateContract(net.Conn) error

	// RetrieveFile is a strict function that enables a client to download a
	// file from a host.
	RetrieveFile(net.Conn) error

	// SetConfig sets the hosting parameters of the host.
	SetConfig(HostSettings)

	// Info returns info about the host, including its hosting parameters, the
	// amount of storage remaining, and the number of active contracts.
	Info() HostInfo
}
