package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// MinerBlockforworkResp is the response struct returned by the API call
// '/miner/blockforwork'.
type MinerBlockforworkResp struct {
	Block      types.Block
	MerkleRoot crypto.Hash
	Target     types.Target
}

// minerBlockforworkHandler handles the API call that retrieves a block for
// work.
func (srv *Server) minerBlockforworkHandler(w http.ResponseWriter, req *http.Request) {
	var bfw MinerBlockforworkResp
	bfw.Block, bfw.MerkleRoot, bfw.Target = srv.miner.BlockForWork()
	writeJSON(w, bfw)
}

// minerStartHandler handles the API call that starts the miner.
func (srv *Server) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	// Scan for the number of threads.
	var threads int
	_, err := fmt.Sscan(req.FormValue("threads"), &threads)
	if err != nil {
		writeError(w, "Malformed number of threads", http.StatusBadRequest)
		return
	}

	srv.miner.SetThreads(threads)
	err = srv.miner.StartMining()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// minerStatusHandler handles the API call that queries the miner's status.
func (srv *Server) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.miner.MinerInfo())
}

// minerStopHandler handles the API call to stop the miner.
func (srv *Server) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	srv.miner.StopMining()
	writeSuccess(w)
}

// minerSubmitBlockHandler handles the API call to submit a block to the miner.
func (srv *Server) minerSubmitBlockHandler(w http.ResponseWriter, req *http.Request) {
	var b types.Block
	err := json.NewDecoder(req.Body).Decode(&b)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
	}

	err = srv.miner.SubmitBlock(b)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
	}
	writeSuccess(w)
}
