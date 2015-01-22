package sia

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	MaxCatchUpBlocks = 100
)

var moreBlocksErr = errors.New("more blocks are available")

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (c *Core) blockHistory() (blockIDs [32]consensus.BlockID) {
	knownBlocks := make([]consensus.BlockID, 0, 32)
	step := consensus.BlockHeight(1)
	for height := c.state.Height(); ; height -= step {
		block, err := c.state.BlockAtHeight(height)
		if err != nil {
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
	genesis, _ := c.state.BlockAtHeight(0)
	knownBlocks = append(knownBlocks, genesis.ID())

	copy(blockIDs[:], knownBlocks)
	return
}

// SendBlocks takes a list of block ids as input, and sends all blocks from
func (c *Core) SendBlocks(knownBlocks [32]consensus.BlockID) (blocks []consensus.Block, err error) {
	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var start consensus.BlockHeight
	for _, id := range knownBlocks {
		height, err := c.state.HeightOfBlock(id)
		if err == nil {
			found = true
			start = height + 1 // start at child
			break
		}
	}
	if !found {
		// The genesis block should be included in knownBlocks - if no matching
		// blocks are found the caller is probably on a different blockchain
		// altogether.
		err = errors.New("no matching block found")
		return
	}

	// Send blocks, starting with the child of the most recent known block.
	for i := start; i < start+MaxCatchUpBlocks; i++ {
		b, err := c.state.BlockAtHeight(i)
		if err != nil {
			break
		}
		blocks = append(blocks, b)
	}

	// If more blocks are available, send a benign error
	if _, maxErr := c.state.BlockAtHeight(start + MaxCatchUpBlocks); maxErr == nil {
		err = moreBlocksErr
	}

	return
}

// CatchUp synchronizes with a peer to acquire any missing blocks. The
// requester sends 32 blocks, starting with the 12 most recent and then
// progressing exponentially backwards to the genesis block. The receiver uses
// these blocks to find the most recent block seen by both peers, and then
// transmits blocks sequentially until the requester is fully synchronized.
func (c *Core) CatchUp(peer network.Address) {
	var newBlocks []consensus.Block
	err := peer.RPC("SendBlocks", c.blockHistory(), &newBlocks)
	if err != nil && err.Error() != moreBlocksErr.Error() {
		// log error
		// TODO: try a different peer?
		return
	}
	for _, block := range newBlocks {
		err = c.state.AcceptBlock(block)
		if err != nil {
			// TODO: something
		}
	}

	// TODO: There is probably a better approach than to call CatchUp
	// recursively. Furthermore, if there is a reorg that's greater than 100
	// blocks, CatchUp is going to fail outright.
	if err != nil && err.Error() == moreBlocksErr.Error() {
		// sleep long enough for state to accept all blocks
		// TODO: this needs to be replaced by a more deterministic wait.
		time.Sleep(time.Second)
		go c.CatchUp(peer)
	}
}
