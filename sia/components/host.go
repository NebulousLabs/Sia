package components

import (
	"net"

	"github.com/NebulousLabs/Sia/consensus"
)

const (
	AcceptContractResponse = "accept"
)

type HostUpdate struct {
	Announcement HostAnnouncement
}

type HostInfo struct {
	Announcement HostAnnouncement

	StorageRemaining int
	ContractCount    int
}

type Host interface {
	// Announce puts an annoucement out so that clients can find the host.
	AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (consensus.Transaction, error)

	HostInfo() (HostInfo, error)

	// NegotiateContract is a strict function that enables a client to
	// communicate with the host to propose a contract.
	//
	// TODO: enhance this documentataion. For now, see the host package for a
	// reference implementation.
	NegotiateContract(conn net.Conn) error

	// RetrieveFile is a strict function that enables a client to download a
	// file from a host.
	RetrieveFile(conn net.Conn) error

	// Returns the number of contracts being managed by the host.
	//
	// TODO: Switch all of this to a status struct.
	NumContracts() int

	// UpdateHost changes the settings used by the host.
	UpdateHost(HostUpdate) error
}
