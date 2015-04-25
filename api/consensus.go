package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

// consensusStatusHandler handles the API call asking for the consensus status.
func (srv *Server) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	currentBlock := srv.cs.CurrentBlock().ID()
	target := srv.cs.CurrentTarget()
	writeJSON(w, struct {
		Height       types.BlockHeight
		CurrentBlock types.BlockID
		Target       types.Target
	}{
		srv.cs.Height(),
		currentBlock,
		target,
	})
}
