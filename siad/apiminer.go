package main

import (
	"fmt"
	"net/http"
)

// Takes a number of threads and then begins mining on that many threads.
func (d *daemon) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	// Scan for the number of threads.
	var threads int
	_, err := fmt.Sscan(req.FormValue("threads"), &threads)
	if err != nil {
		http.Error(w, "Malformed number of threads", 400)
		return
	}

	d.miner.SetThreads(threads)
	err = d.miner.StartMining()
	if err != nil {
		http.Error(w, err.Error(), 500) // TODO: Need to verify that this is the proper error code to be returning.
		return
	}

	writeSuccess(w)
}

// Returns json of the miners status.
func (d *daemon) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	mInfo, err := d.miner.Info()
	if err != nil {
		http.Error(w, "Failed to encode status object", 500)
		return
	}
	writeJSON(w, mInfo)
}

// Calls StopMining() on the core.
func (d *daemon) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	d.miner.StopMining()

	writeSuccess(w)
}
