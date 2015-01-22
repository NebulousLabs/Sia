package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) HostInfo() (components.HostInfo, error) {
	return c.host.HostInfo()
}

// TODO: Make a better UpdateHost thing.
func (c *Core) UpdateHost(announcement components.HostAnnouncement) error {
	info, err := c.host.HostInfo()
	if err != nil {
		return err
	}
	// TODO: This stuff should not happen here, it should be managed by
	// hostSetConfigHandler.
	announcementUpdate := info.Announcement
	announcementUpdate.IPAddress = c.server.Address()
	announcementUpdate.TotalStorage = announcement.TotalStorage
	announcementUpdate.MaxFilesize = announcement.MaxFilesize
	announcementUpdate.Price = announcement.Price
	announcementUpdate.Burn = announcement.Burn

	update := components.HostUpdate{
		Announcement: announcementUpdate,
	}

	return c.host.UpdateHost(update)
}

func (c *Core) AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (err error) {
	_, err = c.host.AnnounceHost(freezeVolume, freezeUnlockHeight)
	if err != nil {
		return
	}
	return
}
