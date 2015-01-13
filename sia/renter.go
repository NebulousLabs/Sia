package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) UpdateRenter() error {
	update := components.RenterUpdate{
		HostDB: c.hostDB,
	}
	return c.renter.UpdateRenter(update)
}
