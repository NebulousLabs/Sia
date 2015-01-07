package components

import (
	"net"

	"github.com/NebulousLabs/Sia/consensus"
)

const (
	AcceptContractResponse = "accept"
)

type HostSettings struct {
	Announcement    HostAnnouncement
	Height          consensus.BlockHeight
	HostDir         string
	TransactionChan chan consensus.Transaction
	Wallet          Wallet
}

type Host interface {
	// Announce puts an annoucement out so that clients can find the host.
	AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (consensus.Transaction, error)

	// NegotiateContract is a strict function that enables a client to
	// communicate with the host to propose a contract. TODO: enhance this
	// documentataion. For now, see the host package for a reference
	// implementation.
	NegotiateContract(conn net.Conn) error

	// UpdateHostSettings changes the settings used by the host.
	UpdateHostSettings(HostSettings) error
}
