package host

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/sia/wallet"
)

type Host interface {
	// Announce puts an annoucement out so that clients can find the host.
	AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (consensus.Transaction, error)

	// UpdateWallet replaces the host's current wallet with a new wallet.
	UpdateWallet(wallet.Wallet) error

	// UpdateSettings changes the settings used by the host.
	UpdateSettings(hostdb.HostAnnouncement) error
}
