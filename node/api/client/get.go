package client

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// GetConsensus requests the /consensus api resource
func (c *Client) GetConsensus() (cg api.ConsensusGET, err error) {
	err = c.Get("/consensus", &cg)
	return
}

// GetMinerHeader uses the /miner/header endpoint to get a header for work.
func (c *Client) GetMinerHeader() (target types.Target, bh types.BlockHeader, err error) {
	targetAndHeader, err := c.GetRawResponse("/miner/header")
	if err != nil {
		return types.Target{}, types.BlockHeader{}, err
	}
	err = encoding.UnmarshalAll(targetAndHeader, &target, &bh)
	return
}
