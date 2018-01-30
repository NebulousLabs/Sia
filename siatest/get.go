package siatest

import "github.com/NebulousLabs/Sia/node/api"

// GetConsensus requests the /consensus api resource
func (node *TestNode) GetConsensus() (cg api.ConsensusGET, err error) {
	err = node.client.Get("/consensus", &cg)
	return
}
