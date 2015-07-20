package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// The ConsensusSetStatus struct contains general information about the
// consensus set, with tags to have idiomatic json encodings.
type ConsensusSetStatus struct {
	Height       types.BlockHeight `json:"height"`
	CurrentBlock types.BlockID     `json:"currentblock"`
	Target       types.Target      `json:"target"`
}

// DEPRECATED
//
// The ConsensusInfo struct contains general information about the consensus
// set.
type ConsensusInfo struct {
	Height       types.BlockHeight
	CurrentBlock types.BlockID
	Target       types.Target
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

// consensusSynchronizeHandlerGET handles a GET request to
// /consensus/synchronize.
func (srv *Server) consensusSynchronizeHandlerGET(w http.ResponseWriter, req *http.Request) {
	peers := srv.gateway.Peers()
	if len(peers) == 0 {
		writeError(w, "No peers available for syncing", http.StatusInternalServerError)
		return
	}
	randPeer, err := crypto.RandIntn(len(peers))
	if err != nil {
		writeError(w, "System error", http.StatusInternalServerError)
	}
	err = srv.cs.Synchronize(peers[randPeer])
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
	}
	writeSuccess(w)
}

// consensusSynchronizeHandler handles the API call asking for the consensus to
// synchronize with other peers.
func (srv *Server) consensusSynchronizeHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.consensusSynchronizeHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /consensus/synchronize", http.StatusBadRequest)
}

// DEPRECATED
//
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
		types.BlockHeight(srv.blockchainHeight),
		srv.currentBlock.ID(),
		currentTarget,
	})
}
