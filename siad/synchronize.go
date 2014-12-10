package main

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	MaxCatchUpBlocks = 100
)

// SendBlocks takes a list of block ids as input, and sends all blocks from
func (e *Environment) SendBlocks(knownBlocks [32]siacore.BlockID, blocks *[]siacore.Block) (err error) {
	e.state.RLock()
	defer e.state.RUnlock()

	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var highest siacore.BlockHeight
	for _, id := range knownBlocks {
		height, err := e.state.HeightOfBlock(id)
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
		b, err := e.state.BlockAtHeight(i)
		if err != nil {
			break
		}
		*blocks = append(*blocks, b)
	}

	return nil
}

// CatchUp synchronizes with a peer to acquire any missing blocks. The
// requester sends 32 blocks, starting with the 12 most recent and then
// progressing exponentially backwards to the genesis block. The receiver uses
// these blocks to find the most recent block seen by both peers, and then
// transmits blocks sequentially until the requester is fully synchronized.
func (e *Environment) CatchUp(peer network.NetAddress) {
	e.state.RLock() // Lock the state while building the block request.
	knownBlocks := make([]siacore.BlockID, 0, 32)
	for i := siacore.BlockHeight(0); i < 12; i++ {
		block, badBlockErr := e.state.BlockAtHeight(e.state.Height() - i)
		if badBlockErr != nil {
			break
		}
		knownBlocks = append(knownBlocks, block.ID())
	}

	backtrace := siacore.BlockHeight(12)
	for i := 12; i < 31; i++ {
		backtrace *= 2
		block, badBlockErr := e.state.BlockAtHeight(e.state.Height() - backtrace)
		if badBlockErr != nil {
			break
		}
		knownBlocks = append(knownBlocks, block.ID())
	}
	// always include the genesis block
	genesis, _ := e.state.BlockAtHeight(0)
	knownBlocks = append(knownBlocks, genesis.ID())
	e.state.RUnlock() // Lock is released once the set of known blocks has been built.

	// prepare for RPC
	var newBlocks []siacore.Block
	var blockArray [32]siacore.BlockID
	copy(blockArray[:], knownBlocks)

	// unlock state during network I/O
	err = peer.RPC("SendBlocks", blockArray, &newBlocks)
	if err != nil {
		fmt.Println(err)
		return
	}

	prevHeight := e.Height()
	for _, block := range newBlocks {
		e.processBlock(block) // processBlock is a blocking function.
	}

	// TODO: There is probably a better approach than to call CatchUp
	// recursively. Furthermore, if there is a reorg that's greater than 100
	// blocks, CatchUp is going to fail outright.
	if prevHeight != e.Height() {
		go e.CatchUp(e.RandomPeer())
	}
}
