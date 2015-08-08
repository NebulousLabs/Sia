package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

// The ConsensusSetStatus struct contains general information about the
// consensus set, with tags to have idiomatic json encodings.
type ConsensusSetStatus struct {
	Height       types.BlockHeight `json:"height"`
	CurrentBlock types.BlockID     `json:"currentblock"`
	Target       types.Target      `json:"target"`
}

// consensusHandlerGET handles a GET request to /consensus.
func (srv *Server) consensusHandlerGET(w http.ResponseWriter, req *http.Request) {
	currentTarget, exists := srv.cs.ChildTarget(srv.currentBlock.ID())
	if build.DEBUG {
		if !exists {
			panic("server has nonexistent current block")
		}
	}

	writeJSON(w, ConsensusSetStatus{
		types.BlockHeight(srv.blockchainHeight),
		srv.currentBlock.ID(),
		currentTarget,
	})
}

// consensusHandler handles the API calls to /consensus.
func (srv *Server) consensusHandler(w http.ResponseWriter, req *http.Request) {
	lockID := srv.mu.RLock()
	defer srv.mu.RUnlock(lockID)

	if req.Method == "" || req.Method == "GET" {
		srv.consensusHandlerGET(w, req)
		return
	}

	writeError(w, "unrecognized method when calling /consensus", http.StatusBadRequest)
}
