package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) UpdateHost(announcement components.HostAnnouncement) error {
	settings := components.HostSettings{
		Announcement:    announcement,
		Height:          c.Height(),
		TransactionChan: c.transactionChan,
		Wallet:          c.wallet,
	}

	return c.host.UpdateHostSettings(settings)
}
