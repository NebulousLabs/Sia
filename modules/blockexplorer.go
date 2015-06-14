package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

// The BlockExplorer interface provides access to the block explorer
type BlockExplorer interface {
	// A wrapper for the ConsensusSet CurrentBlock function. In
	// the future the blockExplorer will store its own version of
	// this block
	CurrentBlock() types.Block

	// Sends notifications when the module updates
	BlockExplorerNotify() <-chan struct{}
}
