package main

import (
	"fmt"
	"net/http"
)

// minerStartHandler handles the api call that starts the miner.
func (d *daemon) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	// Scan for the number of threads.
	var threads int
	_, err := fmt.Sscan(req.FormValue("threads"), &threads)
	if err != nil {
		writeError(w, "Malformed number of threads", 400)
		return
	}

	d.miner.SetThreads(threads)
	err = d.miner.StartMining()
	if err != nil {
		writeError(w, err.Error(), 500) // TODO: Need to verify that this is the proper error code to be returning.
		return
	}

	writeSuccess(w)
}

// minerStatusHandler handles the api call that queries the miner's status.
func (d *daemon) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	mInfo, err := d.miner.Info()
	if err != nil {
		writeError(w, "Failed to encode status object", 500)
		return
	}
	writeJSON(w, mInfo)
}

// minerStopHandler handles the api call to stop the miner.
func (d *daemon) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	d.miner.StopMining()

	writeSuccess(w)
}
