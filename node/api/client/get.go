package client

import "github.com/NebulousLabs/Sia/node/api"

// GetConsensus requests the /consensus api resource
func (c *Client) GetConsensus() (cg api.ConsensusGET, err error) {
	err = c.Get("/consensus", &cg)
	return
}
