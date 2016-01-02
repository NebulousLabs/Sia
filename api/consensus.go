package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

// ConsensusGET contains general information about the consensus set, with tags
// to support idiomatic json encodings.
type ConsensusGET struct {
	Height       types.BlockHeight `json:"height"`
	CurrentBlock types.BlockID     `json:"currentblock"`
	Target       types.Target      `json:"target"`
}

// ConsensusBlockGET contains a block.
type ConsensusBlockGET struct {
	Block types.Block `json:"block"`
}

// consensusHandler handles the API calls to /consensus.
func (srv *Server) consensusHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	cbid := srv.cs.CurrentBlock().ID()
	currentTarget, _ := srv.cs.ChildTarget(cbid)
	writeJSON(w, ConsensusGET{
		Height:       srv.cs.Height(),
		CurrentBlock: cbid,
		Target:       currentTarget,
	})
}

// consensusBlockHandler handles the API calls to /consensus/blocks/:height.
func (srv *Server) consensusBlocksHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	var height types.BlockHeight
	_, err := fmt.Sscan(ps.ByName("height"), &height)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	block, exists := srv.cs.BlockAtHeight(height)
	if !exists {
		writeError(w, "no block found at given height", http.StatusBadRequest)
		return
	}
	writeJSON(w, ConsensusBlockGET{
		Block: block,
	})
}
