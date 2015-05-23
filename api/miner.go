package api

import (
	"fmt"
	"net/http"
)

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

// minerGetWorkHandler handles the API call that gets work for an external miner.
func (srv *Server) minerGetWorkHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.miner.GetWork())
}
