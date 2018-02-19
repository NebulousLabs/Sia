package client

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// MinerHeaderGet uses the /miner/header endpoint to get a header for work.
func (c *Client) MinerHeaderGet() (target types.Target, bh types.BlockHeader, err error) {
	targetAndHeader, err := c.getRawResponse("/miner/header")
	if err != nil {
		return types.Target{}, types.BlockHeader{}, err
	}
	err = encoding.UnmarshalAll(targetAndHeader, &target, &bh)
	return
}

// MinerHeaderPost uses the /miner/header endpoint to submit a solved block
// header that was previously received from the same endpoint
func (c *Client) MinerHeaderPost(bh types.BlockHeader) (err error) {
	err = c.post("/miner/header", string(encoding.Marshal(bh)), nil)
	return
}
