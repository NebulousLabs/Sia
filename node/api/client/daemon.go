package client

import "github.com/NebulousLabs/Sia/node/api"

// DaemonVersionGet requests the /daemon/version resource
func (c *Client) DaemonVersionGet() (dvg api.DaemonVersionGet, err error) {
	err = c.get("/daemon/version", &dvg)
	return
}

// DaemonStopGet stops the daemon using the /daemon/stop endpoint.
func (c *Client) DaemonStopGet() (err error) {
	err = c.get("/daemon/stop", nil)
	return
}

// DaemonUpdateGet checks for an available daemon update.
func (c *Client) DaemonUpdateGet() (dig api.DaemonUpdateGet, err error) {
	err = c.get("/daemon/update", nil)
	return
}

// DaemonUpdatePost updates the daemon.
func (c *Client) DaemonUpdatePost() (err error) {
	err = c.post("/daemon/update", "", nil)
	return
}
