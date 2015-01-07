package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) UpdateRenter() error {
	settings := components.RenterSettings{
		HostDB: c.hostDB,
	}
	return c.renter.UpdateRenterSettings(settings)
}
