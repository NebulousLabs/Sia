package gateway

import (
	"github.com/NebulousLabs/Sia/consensus"
)

func (g *Gateway) update() {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, appliedBlockIDs, err := g.state.BlocksSince(g.latestBlock)
	if err != nil {
		if consensus.DEBUG {
			panic(err)
		}
	}
	if len(appliedBlockIDs) == 0 {
		// This should never happen, but for some of the tests the gateway will
		// end up with a notification that it doesn't own. This doesn't seem to
		// be a problem for the host, only the gateway.
		return
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
}

func (g *Gateway) threadedConsensusListen(consensusChan chan struct{}) {
	for _ = range consensusChan {
		g.update()
	}
}
