package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) HostInfo(info components.HostInfo) (components.HostInfo, error) {
	return c.host.HostInfo()
}

// TODO: Make a better UpdateHost thing.
func (c *Core) UpdateHost(announcement components.HostAnnouncement) error {
	update := components.HostUpdate{
		Announcement:    announcement,
		Height:          c.Height(),
		HostDir:         c.hostDir,
		TransactionChan: c.transactionChan,
		Wallet:          c.wallet,
	}

	return c.host.UpdateHost(update)
}
