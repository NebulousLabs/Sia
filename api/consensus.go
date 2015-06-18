package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

type ConsensusInfo struct {
	Height       types.BlockHeight
	CurrentBlock types.BlockID
	Target       types.Target
}

// consensusStatusHandler handles the API call asking for the consensus status.
func (srv *Server) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	lockID := srv.mu.RLock()
	defer srv.mu.RUnlock(lockID)

	currentTarget, exists := srv.cs.ChildTarget(srv.currentBlock.ID())
	if build.DEBUG {
		if !exists {
			panic("server has nonexistent current block")
		}
	}

	writeJSON(w, ConsensusInfo{
		srv.blockchainHeight,
		srv.currentBlock.ID(),
		currentTarget,
	})
}

// consensusSynchronizeHandler handles the API call asking for the consensus to
// synchronize with other peers.
func (srv *Server) consensusSynchronizeHandler(w http.ResponseWriter, req *http.Request) {
	peers := srv.gateway.Peers()
	if len(peers) == 0 {
		writeError(w, "No peers available for syncing", http.StatusInternalServerError)
		return
	}

	// TODO: How should this be handled? Multiple simultaneous peers? First
	// peer is a bad method.
	go srv.cs.Synchronize(peers[0])

	writeSuccess(w)
}
