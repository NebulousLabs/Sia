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
	ClientPayout       consensus.Currency // client contribution towards payout each window
	HostPayout         consensus.Currency // host contribution towards payout each window
	ValidProofAddress  consensus.CoinAddress
	MissedProofAddress consensus.CoinAddress
}

type HostInfo struct {
	HostSettings

	StorageRemaining int64
	NumContracts     int
}

type Host interface {
	// Announce announces the host on the blockchain. A host announcement
	// requires two things: the host's address, and a volume of "frozen"
	// (time-locked) coins, used to mitigate Sybil attacks.
	Announce(network.Address, consensus.Currency, consensus.BlockHeight) error

	// NegotiateContract is a strict function that enables a client to
	// communicate with the host to propose a contract.
	//
	// TODO: enhance this documentataion. For now, see the host package for a
	// reference implementation.
	NegotiateContract(net.Conn) error

	// RetrieveFile is a strict function that enables a client to download a
	// file from a host.
	RetrieveFile(net.Conn) error

	// Returns the number of contracts being managed by the host.
	//
	// TODO: Switch all of this to a status struct.
	NumContracts() int
}
