package components

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type HostSettings struct {
	Announcement    HostAnnouncement
	Height          consensus.BlockHeight
	TransactionChan chan consensus.Transaction
	Wallet          Wallet
}

type Host interface {
	// Announce puts an annoucement out so that clients can find the host.
	AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (consensus.Transaction, error)

	// UpdateHostSettings changes the settings used by the host.
	UpdateHostSettings(HostSettings) error
}
