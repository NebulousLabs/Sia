package components

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type HostSettings struct {
	Announcement    HostAnnouncement
	Wallet          Wallet
	TransactionChan chan consensus.Transaction
}

type Host interface {
	// Announce puts an annoucement out so that clients can find the host.
	AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (consensus.Transaction, error)

	// UpdateWallet replaces the host's current wallet with a new wallet.
	UpdateWallet(Wallet) error

	// UpdateHostSettings changes the settings used by the host.
	UpdateHostSettings(HostSettings) error
}
