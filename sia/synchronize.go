package sia

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	MaxCatchUpBlocks = 100
)

// SendBlocks takes a list of block ids as input, and sends all blocks from
func (c *Core) SendBlocks(knownBlocks [32]consensus.BlockID) (blocks []consensus.Block, err error) {
	c.state.RLock()
	defer c.state.RUnlock()

	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var highest consensus.BlockHeight
	for _, id := range knownBlocks {
		height, err := c.state.HeightOfBlock(id)
		if err == nil {
			found = true
			if height > highest {
				highest = height
			}
		}
	}
	if !found {
		// The genesis block should be included in knownBlocks - if no matching
		// blocks are found the caller is probably on a different blockchain
		// altogether.
		err = errors.New("no matching block found")
		return
	}

	// Send over all blocks from the first known block.
	for i := highest; i < highest+MaxCatchUpBlocks; i++ {
		b, err := c.state.BlockAtHeight(i)
		if err != nil {
			break
		}
		blocks = append(blocks, b)
	}

	return
}

// CatchUp synchronizes with a peer to acquire any missing blocks. The
// requester sends 32 blocks, starting with the 12 most recent and then
// progressing exponentially backwards to the genesis block. The receiver uses
// these blocks to find the most recent block seen by both peers, and then
// transmits blocks sequentially until the requester is fully synchronized.
func (c *Core) CatchUp(peer network.Address) {
	c.state.RLock() // Lock the state while building the block request.
	knownBlocks := make([]consensus.BlockID, 0, 32)
	for i := consensus.BlockHeight(0); i < 12; i++ {
		block, badBlockErr := c.state.BlockAtHeight(c.state.Height() - i)
		if badBlockErr != nil {
			break
		}
		knownBlocks = append(knownBlocks, block.ID())
	}

	backtrace := consensus.BlockHeight(12)
	for i := 12; i < 31; i++ {
		backtrace *= 2
		block, badBlockErr := c.state.BlockAtHeight(c.state.Height() - backtrace)
		if badBlockErr != nil {
			break
		}
		knownBlocks = append(knownBlocks, block.ID())
	}
	// always include the genesis block
	genesis, _ := c.state.BlockAtHeight(0)
	knownBlocks = append(knownBlocks, genesis.ID())
	c.state.RUnlock() // Lock is released once the set of known blocks has been built.

	// prepare for RPC
	var newBlocks []consensus.Block
	var blockArray [32]consensus.BlockID
	copy(blockArray[:], knownBlocks)

	// unlock state during network I/O
	err := peer.RPC("SendBlocks", blockArray, &newBlocks)
	if err != nil {
		// log error
		return
	}

	prevHeight := c.Height()
	for _, block := range newBlocks {
		c.processBlock(block) // processBlock is a blocking function.
	}

	// TODO: There is probably a better approach than to call CatchUp
	// recursively. Furthermore, if there is a reorg that's greater than 100
	// blocks, CatchUp is going to fail outright.
	if prevHeight != c.Height() {
		go c.CatchUp(c.RandomPeer())
	}
}
