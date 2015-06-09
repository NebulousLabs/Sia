package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

type BlockData struct {
	Timestamp types.Timestamp // The timestamp on the block
	Target    types.Target    // The target the block was mined for
	Size      uint64          // The size in bytes of the marshalled block
}

// The BlockExplorer interface provides access to the block explorer
type BlockExplorer interface {
	// A wrapper for the ConsensusSet CurrentBlock function. In
	// the future the blockExplorer will store its own version of
	// this block
	CurrentBlock() types.Block

	// Sends notifications when the module updates
	BlockExplorerNotify() <-chan struct{}

	// Function to populate and return an instance of the ExplorerInfo struct
}
