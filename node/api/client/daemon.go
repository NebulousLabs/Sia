package client

import "github.com/NebulousLabs/Sia/node/api"

// DaemonVersionGet requests the /daemon/version resource
func (c *Client) DaemonVersionGet() (dvg api.DaemonVersionGet, err error) {
	err = c.get("/daemon/version", &dvg)
	return
}
