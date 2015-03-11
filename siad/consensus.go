package main

import (
	"net/http"
)

// consensusStatusHandler handles the API call asking for the consensus status.
func (d *daemon) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	currentBlock := d.state.CurrentBlock().ID()
	target := d.state.CurrentTarget()
	writeJSON(w, struct {
		Height       int
		CurrentBlock string
		Target       string
	}{
		int(d.state.Height()),
		string(currentBlock[:]),
		string(target[:]),
	})
}
