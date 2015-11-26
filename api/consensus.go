package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/types"
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

// consensusHandlerGET handles a GET request to /consensus.
func (srv *Server) consensusHandlerGET(w http.ResponseWriter, req *http.Request) {
	id := srv.mu.RLock()
	defer srv.mu.RUnlock(id)

	cbid := srv.cs.CurrentBlock().ID()
	currentTarget, _ := srv.cs.ChildTarget(cbid)
	writeJSON(w, ConsensusGET{
		Height:       srv.cs.Height(),
		CurrentBlock: cbid,
		Target:       currentTarget,
	})
}

// consensusHandler handles the API calls to /consensus.
func (srv *Server) consensusHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.consensusHandlerGET(w, req)
	} else {
		writeError(w, "unrecognized method when calling /consensus", http.StatusBadRequest)
	}
}

// consensusBlockHandlerGET handles a GET request to /consensus/block.
func (srv *Server) consensusBlockHandlerGET(w http.ResponseWriter, req *http.Request) {
	var height types.BlockHeight
	_, err := fmt.Sscan(req.FormValue("height"), &height)
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

// consensusBlockHandler handles the API calls to /consensus/block.
func (srv *Server) consensusBlockHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.consensusBlockHandlerGET(w, req)
	} else {
		writeError(w, "unrecognized method when calling /consensus/block", http.StatusBadRequest)
	}
}
