package client

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// ConsensusGet requests the /consensus api resource
func (c *Client) ConsensusGet() (cg api.ConsensusGET, err error) {
	err = c.Get("/consensus", &cg)
	return
}

// Gateway requests the /gateway api resource
func (c *Client) GatewayGet() (gwg api.GatewayGET, err error) {
	err = c.Get("/gateway", &gwg)
	return
}

// MinerHeaderGet uses the /miner/header endpoint to get a header for work.
func (c *Client) MinerHeaderGet() (target types.Target, bh types.BlockHeader, err error) {
	targetAndHeader, err := c.GetRawResponse("/miner/header")
	if err != nil {
		return types.Target{}, types.BlockHeader{}, err
	}
	err = encoding.UnmarshalAll(targetAndHeader, &target, &bh)
	return
}
