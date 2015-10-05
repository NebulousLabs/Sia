package api

import (
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

	curblockID := srv.currentBlock.ID()
	currentTarget, _ := srv.cs.ChildTarget(curblockID)
	writeJSON(w, ConsensusGET{
		Height:       types.BlockHeight(srv.blockchainHeight),
		CurrentBlock: srv.currentBlock.ID(),
		Target:       currentTarget,
	})
}

// consensusHandler handles the API calls to /consensus.
func (srv *Server) consensusHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.consensusHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /consensus", http.StatusBadRequest)
}

// consensusBlockHandlerGET handles a GET request to /consensus/block.
func (srv *Server) consensusBlockHandlerGET(w http.ResponseWriter, req *http.Request) {
	height, err := scanBlockHeight(req.FormValue("height"))
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
		return
	}
	writeError(w, "unrecognized method when calling /consensus/block", http.StatusBadRequest)
}
