package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

// The ConsensusGET struct contains general information about the consensus
// set, with tags to support idiomatic json encodings.
type ConsensusGET struct {
	Height       types.BlockHeight `json:"Height"`
	CurrentBlock types.BlockID     `json:"CurrentBlock"`
	Target       types.Target      `json:"Target"`
}

// consensusHandlerGET handles a GET request to /consensus.
func (srv *Server) consensusHandlerGET(w http.ResponseWriter, req *http.Request) {
	id := srv.mu.RLock()
	defer srv.mu.RUnlock(id)

	curblockID := srv.currentBlock.ID()
	currentTarget, exists := srv.cs.ChildTarget(curblockID)
	if build.DEBUG {
		if !exists {
			fmt.Printf("Could not find block %s\n", curblockID)
			panic("server has nonexistent current block")
		}
	}

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
	} else {
		writeError(w, "unrecognized method when calling /consensus", http.StatusBadRequest)
	}
}
