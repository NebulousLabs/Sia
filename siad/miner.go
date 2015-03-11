package main

import (
	"fmt"
	"net/http"
)

// minerStartHandler handles the API call that starts the miner.
func (d *daemon) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	// Scan for the number of threads.
	var threads int
	_, err := fmt.Sscan(req.FormValue("threads"), &threads)
	if err != nil {
		writeError(w, "Malformed number of threads", http.StatusBadRequest)
		return
	}

	d.miner.SetThreads(threads)
	err = d.miner.StartMining()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// minerStatusHandler handles the API call that queries the miner's status.
func (d *daemon) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	mInfo, err := d.miner.Info()
	if err != nil {
		writeError(w, "Failed to encode status object", http.StatusInternalServerError)
		return
	}
	writeJSON(w, mInfo)
}

// minerStopHandler handles the API call to stop the miner.
func (d *daemon) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	d.miner.StopMining()
	writeSuccess(w)
}
