package main

import (
	"net/http"
)

func (d *daemon) updateCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := checkForUpdate()
	if err != nil {
		http.Error(w, err.Error(), 500)
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
		http.Error(w, err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.core.StateInfo())
}

func (d *daemon) stopHandler(w http.ResponseWriter, req *http.Request) {
	writeSuccess(w)

	// send stop signal
	d.srv.Stop(1e9)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	if len(d.core.AddressBook()) == 0 {
		http.Error(w, "No peers available for syncing", 500)
		return
	}

	go d.core.CatchUp(d.core.RandomPeer())

	writeSuccess(w)
}
