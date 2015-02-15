package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/network"
)

func (d *daemon) peerAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("addr"))
	err := d.network.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) peerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("addr"))
	err := d.network.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

func (d *daemon) peerStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.network.AddressBook())
}
