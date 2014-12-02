package siad

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
	e.state.Lock()
	defer e.state.Unlock()

	// Find the most recent block that is in our current path. Since
	// knownBlocks is ordered from newest to oldest, we can break as soon as
	// we find a match.
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
func (e *Environment) CatchUp(peer network.NetAddress) (err error) {
	e.state.Lock()
	defer e.state.Unlock()

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

	// prepare for RPC
	var newBlocks []siacore.Block
	var blockArray [32]siacore.BlockID
	copy(blockArray[:], knownBlocks)

	// unlock state during network I/O
	e.state.Unlock()
	err = peer.RPC("SendBlocks", blockArray, &newBlocks)
	e.state.Lock()
	if err != nil {
		return err
	}

	prevHeight := e.state.Height()

	for i := range newBlocks {
		if err = e.state.AcceptBlock(newBlocks[i]); err != nil {
			if err != siacore.BlockKnownErr && err != siacore.FutureBlockErr {
				// Return if there's an error, but don't return for benign
				// errors: BlockKnownErr and FutureBlockErr are both benign.
				return err
			}
		}
	}

	// recurse until the height stops increasing
	if prevHeight != e.state.Height() {
		e.state.Unlock()
		err = e.CatchUp(peer) // Prevents deadlock, while keeping the defer.
		e.state.Lock()
		return
	}

	return nil
}
