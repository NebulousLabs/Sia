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

	d.core.UpdateMiner(threads)
	d.core.StartMining()
	fmt.Fprintf(w, "Now mining on %v threads.", threads)
}

// Calls StopMining() on the core.
func (d *daemon) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	d.core.StopMining()
	fmt.Fprintf(w, "Turning off mining.")
}

// Returns json of the miners status.
func (d *daemon) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	json, err := d.core.MinerInfo()
	if err != nil {
		http.Error(w, "Failed to encode status object", 500)
		return
	}
	w.Write(json)
}
