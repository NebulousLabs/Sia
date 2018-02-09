package client

import (
	"strconv"

	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"
)

// ConsensusGet requests the /consensus api resource
func (c *Client) ConsensusGet() (cg api.ConsensusGET, err error) {
	err = c.get("/consensus", &cg)
	return
}

// ConsensusBlocksIDGet requests the /consensus/blocks/:id api resource
func (c *Client) ConsensusBlocksIDGet(id types.BlockID) (block types.Block, err error) {
	err = c.Get("/consensus/blocks/"+id.String(), &block)
	return
}

// ConsensusHeadersHeightGet requests the /consensus/headers/:height api resource
func (c *Client) ConsensusHeadersHeightGet(height types.BlockHeight) (chg api.ConsensusHeadersGET, err error) {
	err = c.Get("/consensus/headers/"+strconv.FormatUint(uint64(height), 10), &chg)
	return
}
