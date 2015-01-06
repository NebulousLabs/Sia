package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) UpdateHost(announcement components.HostAnnouncement) error {
	settings := components.HostSettings{
		Announcement:    announcement,
		Wallet:          c.wallet,
		TransactionChan: c.transactionChan,
	}

	return c.host.UpdateHostSettings(settings)
}
