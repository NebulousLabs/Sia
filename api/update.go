package api

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ReceiveConsensusSetUpdate gets called by the consensus set every time there
// is a change to the blockchain.
func (srv *Server) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := srv.mu.Lock()
	defer srv.mu.Unlock(lockID)

	srv.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	srv.blockchainHeight += types.BlockHeight(len(cc.AppliedBlocks))
	srv.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1]
}
