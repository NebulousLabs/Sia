package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

type BlockExplorer interface {
	// A wrapper for the ConsensusSet CurrentBlock function. In
	// the future the blockExplorer will store its own version of
	// this block
	CurrentBlock() types.Block
}
