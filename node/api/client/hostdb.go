package client

import (
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// HostDbGet requests the /hostdb endpoint's resources.
func (c *Client) HostDbGet() (hdg api.HostdbGet, err error) {
	err = c.get("/hostdb", &hdg)
	return
}

// HostDbActiveGet requests the /hostdb/active endpoint's resources.
func (c *Client) HostDbActiveGet() (hdag api.HostdbActiveGET, err error) {
	err = c.get("/hostdb/active", &hdag)
	return
}

// HostDbAllGet requests the /hostdb/all endpoint's resources.
func (c *Client) HostDbAllGet() (hdag api.HostdbAllGET, err error) {
	err = c.get("/hostdb/all", &hdag)
	return
}

// HostDbHostsGet request the /hostdb/hosts/:pubkey endpoint's resources.
func (c *Client) HostDbHostsGet(pk types.SiaPublicKey) (hhg api.HostdbHostsGET, err error) {
	err = c.get("/hostdb/hosts/"+pk.String(), &hhg)
	return
}
