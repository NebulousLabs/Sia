package gateway

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	MaxCatchUpBlocks = 100
)

var (
	moreBlocksErr = errors.New("more blocks are available")
)

// Sychronize to synchronize the local consensus set (i.e. the blockchain) with
// the network consensus set.
// TODO: don't run two Synchronize threads at the same time
func (g *Gateway) Synchronize() (err error) {
	peer, err := g.randomPeer()
	if err != nil {
		return
	}
	go g.synchronize(peer)
	return
}

// synchronize asks a peer for new blocks. The requester sends 32 block IDs,
// starting with the 12 most recent and then progressing exponentially
// backwards to the genesis block. The receiver uses these blocks to find the
// most recent block seen by both peers. From this starting height, it
// transmits blocks sequentially. Multiple such transmissions may be required
// to fully synchronize.
func (g *Gateway) synchronize(peer network.Address) {
	var newBlocks []consensus.Block
	err := peer.RPC("SendBlocks", g.blockHistory(), &newBlocks)
	if err != nil && err.Error() != moreBlocksErr.Error() {
		// log error
		// TODO: try a different peer?
		return
	}
	for _, block := range newBlocks {
		acceptErr := g.state.AcceptBlock(block)
		if acceptErr != nil {
			// TODO: something
			//
			// TODO: If the error is a FutureTimestampErr, need to wait before trying the
			// block again.
		}
	}

	// TODO: There is probably a better approach than to call CatchUp
	// recursively. Furthermore, if there is a reorg that's greater than 100
	// blocks, CatchUp is going to fail outright.
	if err != nil && err.Error() == moreBlocksErr.Error() {
		go g.synchronize(peer)
	}
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (g *Gateway) blockHistory() (blockIDs [32]consensus.BlockID) {
	knownBlocks := make([]consensus.BlockID, 0, 32)
	step := consensus.BlockHeight(1)
	for height := g.state.Height(); ; height -= step {
		block, exists := g.state.BlockAtHeight(height)
		if !exists {
			// faulty state; log high-priority error
			return
		}
		knownBlocks = append(knownBlocks, block.ID())

		// after 12, start doubling
		if len(knownBlocks) >= 12 {
			step *= 2
		}

		// this check has to come before height -= step;
		// otherwise we might underflow
		if height <= step {
			break
		}
	}

	// always include the genesis block
	genesis, exists := g.state.BlockAtHeight(0)
	if !exists {
		// this should never happen
		return
	}
	knownBlocks = append(knownBlocks, genesis.ID())

	copy(blockIDs[:], knownBlocks)
	return
}

// SendBlocks returns a sequential set of blocks based on the 32 input block
// IDs. The most recent known ID is used as the starting point, and up to
// 'MaxCatchUpBlocks' from that BlockHeight onwards are returned. If more
// blocks could be returned, a 'moreBlocksErr' will be returned as well.
func (g *Gateway) SendBlocks(knownBlocks [32]consensus.BlockID) (blocks []consensus.Block, err error) {
	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var start consensus.BlockHeight
	for _, id := range knownBlocks {
		height, exists := g.state.HeightOfBlock(id)
		if !exists {
			found = true
			start = height + 1 // start at child
			break
		}
	}
	if !found {
		// The genesis block should be included in knownBlocks - if no matching
		// blocks are found, the caller is probably on a different blockchain
		// altogether.
		err = errors.New("no matching block found")
		return
	}

	// Send blocks, starting with the child of the most recent known block.
	// TODO: use BlocksSince instead?
	for i := start; i < start+MaxCatchUpBlocks; i++ {
		b, exists := g.state.BlockAtHeight(i)
		if !exists {
			break
		}
		blocks = append(blocks, b)
	}

	// If more blocks are available, send a benign error
	if _, exists := g.state.BlockAtHeight(start + MaxCatchUpBlocks); !exists {
		err = moreBlocksErr
	}

	return
}
