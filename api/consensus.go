package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

// ConsensusGET contains general information about the consensus set, with tags
// to support idiomatic json encodings.
type ConsensusGET struct {
	Synced       bool              `json:"synced"`
	Height       types.BlockHeight `json:"height"`
	CurrentBlock types.BlockID     `json:"currentblock"`
	Target       types.Target      `json:"target"`
}

// consensusHandler handles the API calls to /consensus.
func (srv *Server) consensusHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	cbid := srv.cs.CurrentBlock().ID()
	currentTarget, _ := srv.cs.ChildTarget(cbid)
	writeJSON(w, ConsensusGET{
		Synced:       srv.cs.Synced(),
		Height:       srv.cs.Height(),
		CurrentBlock: cbid,
		Target:       currentTarget,
	})
}
