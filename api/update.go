package api

import (
	"github.com/NebulousLabs/Sia/types"
)

// ReceiveConsensusSetUpdate gets called by the consensus set every time there
// is a change to the blockchain.
func (srv *Server) ReceiveConsensusSetUpdate(revertedBlocks, appliedBlocks []types.Block) {
	lockID := srv.mu.Lock()
	defer srv.mu.Unlock(lockID)

	srv.blockchainHeight -= types.BlockHeight(len(revertedBlocks))
	srv.blockchainHeight += types.BlockHeight(len(appliedBlocks))
	srv.currentBlock = appliedBlocks[len(appliedBlocks)-1]
}
