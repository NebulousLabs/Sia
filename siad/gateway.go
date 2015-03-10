package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

func (d *daemon) gatewayAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("addr"))
	err := d.gateway.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) gatewayRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("addr"))
	err := d.gateway.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) gatewaySyncHandler(w http.ResponseWriter, req *http.Request) {
	err := d.gateway.Synchronize()
	if err != nil {
		writeError(w, "No peers available for syncing", 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) gatewayStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.gateway.Info())
}
