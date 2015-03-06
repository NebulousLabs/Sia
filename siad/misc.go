package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	stateInfo := consensus.StateInfo{
		CurrentBlock: d.state.CurrentBlock().ID(),
		Height:       d.state.Height(),
		Target:       d.state.CurrentTarget(),
	}
	writeJSON(w, stateInfo)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	err := d.gateway.Synchronize()
	if err != nil {
		writeError(w, "No peers available for syncing", 500)
		return
	}

	writeSuccess(w)
}
