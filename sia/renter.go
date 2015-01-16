package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (c *Core) RenterDownload(nickname, filepath string) error {
	return c.renter.Download(nickname, filepath)
}

func (c *Core) RentInfo() (components.RentInfo, error) {
	return c.renter.RentInfo()
}

func (c *Core) RenameFile(currentName, newName string) error {
	return c.renter.RenameFile(currentName, newName)
}

func (c *Core) RentFile(params components.RentFileParameters) error {
	return c.renter.RentFile(params)
}

func (c *Core) RentSmallFile(params components.RentSmallFileParameters) error {
	return c.renter.RentSmallFile(params)
}
