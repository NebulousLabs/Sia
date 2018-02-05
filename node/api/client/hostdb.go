package client

import "github.com/NebulousLabs/Sia/node/api"

// HostDbActiveGet requests the /hostdb/active endpoint's resources
func (c *Client) HostDbActiveGet() (hdag api.HostdbActiveGET, err error) {
	err = c.Get("/hostdb/active", &hdag)
	return
}
