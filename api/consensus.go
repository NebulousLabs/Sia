package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

type ConsensusInfo struct {
	Height       types.BlockHeight
	CurrentBlock types.BlockID
	Target       types.Target
}

// consensusStatusHandler handles the API call asking for the consensus status.
func (srv *Server) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, ConsensusInfo{
		srv.cs.Height(),
		srv.cs.CurrentBlock().ID(),
		srv.cs.CurrentTarget(),
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
	go srv.cs.Synchronize(peers[0])

	writeSuccess(w)
}
