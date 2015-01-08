package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) UpdateHost(announcement components.HostAnnouncement) error {
	update := components.HostUpdate{
		Announcement:    announcement,
		Height:          c.Height(),
		HostDir:         c.hostDir,
		State:           c,
		TransactionChan: c.transactionChan,
		Wallet:          c.wallet,
	}

	return c.host.UpdateHost(update)
}
