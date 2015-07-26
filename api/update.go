package api

import (
	"github.com/NebulousLabs/Sia/modules"
)

// ProcessConsensusChange gets called by the consensus set every time there is
// a change to the blockchain.
func (srv *Server) ProcessConsensusChange(cc modules.ConsensusChange) {
	lockID := srv.mu.Lock()
	defer srv.mu.Unlock(lockID)

	srv.blockchainHeight -= len(cc.RevertedBlocks)
	srv.blockchainHeight += len(cc.AppliedBlocks)
	srv.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]
}
