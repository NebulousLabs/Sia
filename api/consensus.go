package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// consensusStatusHandler handles the API call asking for the consensus status.
func (srv *Server) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	currentBlock := srv.state.CurrentBlock().ID()
	target := srv.state.CurrentTarget()
	writeJSON(w, struct {
		Height       consensus.BlockHeight
		CurrentBlock consensus.BlockID
		Target       consensus.Target
	}{
		srv.state.Height(),
		currentBlock,
		target,
	})
}
