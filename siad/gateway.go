package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

func (d *daemon) peerAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.Address(req.FormValue("addr"))
	err := d.gateway.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) peerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.Address(req.FormValue("addr"))
	err := d.gateway.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) peerStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.gateway.Info())
}
