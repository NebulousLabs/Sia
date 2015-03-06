package main

import (
	"net/http"
)

// consensusStatusHandler returns information about the consensus module.
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
