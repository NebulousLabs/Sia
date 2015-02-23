package gateway

import (
	"github.com/NebulousLabs/Sia/consensus"
)

func (g *Gateway) threadedConsensusListen(consensusChan chan struct{}) {
	for _ = range consensusChan {
		g.mu.Lock()

		_, appliedBlockIDs, err := g.state.BlocksSince(g.latestBlock)
		if err != nil {
			if consensus.DEBUG {
				panic(err)
			}
		}
		if len(appliedBlockIDs) == 0 {
			println("I DID A CONTINUE")
			g.mu.Unlock()
			continue
		}
		g.latestBlock = appliedBlockIDs[len(appliedBlockIDs)-1]

		for _, id := range appliedBlockIDs {
			block, exists := g.state.Block(id)
			if !exists {
				if consensus.DEBUG {
					panic("block doesn't exist but was returned by BlocksSince")
				}
				continue
			}

			g.RelayBlock(block)
		}

		g.mu.Unlock()
	}
}
