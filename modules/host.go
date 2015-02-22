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
	FileSize           uint64
	StartHeight        consensus.BlockHeight
	WindowSize         consensus.BlockHeight // how many blocks a host has to submit each proof
	NumWindows         uint64
	Price              consensus.Currency // client contribution towards payout each window
	Collateral         consensus.Currency // host contribution towards payout each window
	ValidProofAddress  consensus.UnlockHash
	MissedProofAddress consensus.UnlockHash
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
