package renter

import (
	"github.com/NebulousLabs/Sia/types"
)

// ReceiveConsensusSetUpdate will be called by the consensus set every time
// there is a change in the blockchain. Updates will always be called in order.
func (r *Renter) ReceiveConsensusSetUpdate(revertedBlocks []types.Block, appliedBlocks []types.Block) {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)
	r.blockHeight -= types.BlockHeight(len(revertedBlocks))
	r.blockHeight += types.BlockHeight(len(appliedBlocks))
	r.updateSubscribers()
}
