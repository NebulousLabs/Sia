package siad

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	MaxCatchUpBlocks = 100
)

// SendBlocks sends all known blocks from the given height forward from the
// longest known fork.
func (e *Environment) SendBlocks(knownBlocks [32]siacore.BlockID, blocks *[]siacore.Block) error {
	e.state.Lock()
	defer e.state.Unlock()

	// Find the most recent block that is in our current path. Since
	// knownBlocks is ordered from newest to oldest, we can break as soon as
	// we find a match.
	var blockNode *siacore.BlockNode
	for i := range knownBlocks {
		blockNode := e.state.NodeFromID(knownBlocks[i])
		if blockNode != nil {
			break
		}
	}
	if blockNode == nil {
		return errors.New("no matching block found")
	}

	for i := blockNode.Height; i < blockNode.Height+MaxCatchUpBlocks; i++ {
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
		return e.CatchUp(peer)
	}

	return nil
}
