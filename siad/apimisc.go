package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

func (d *daemon) updateCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := checkForUpdate()
	if err != nil {
		writeError(w, err.Error(), 500)
		return
	}

	writeJSON(w, struct {
		Available bool
		Version   string
	}{available, version})
}

func (d *daemon) updateApplyHandler(w http.ResponseWriter, req *http.Request) {
	err := applyUpdate(req.FormValue("version"))
	if err != nil {
		writeError(w, err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	stateInfo := consensus.StateInfo{
		CurrentBlock: d.state.CurrentBlock().ID(),
		Height:       d.state.Height(),
		Target:       d.state.CurrentTarget(),
	}
	writeJSON(w, stateInfo)
}

func (d *daemon) stopHandler(w http.ResponseWriter, req *http.Request) {
	writeSuccess(w)

	// send stop signal
	d.apiServer.Stop(1e9)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	peer, err := d.network.RandomPeer()
	if err != nil {
		writeError(w, "No peers available for syncing", 500)
		return
	}

	go d.CatchUp(peer)

	writeSuccess(w)
}
