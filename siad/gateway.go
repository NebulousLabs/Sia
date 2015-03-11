package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

// gatewayStatusHandler handles the API call asking for the gatway status.
func (d *daemon) gatewayStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.gateway.Info())
}

// gatewaySynchronizeHandler handles the API call asking for the gateway to
// synchronize with other peers.
func (d *daemon) gatewaySynchronizeHandler(w http.ResponseWriter, req *http.Request) {
	err := d.gateway.Synchronize()
	if err != nil {
		writeError(w, "No peers available for syncing", http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// gatewayPeerAddHandler handles the API call to add a peer to the gateway.
func (d *daemon) gatewayPeerAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("address"))
	err := d.gateway.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// gatewayPeerRemoveHandler handles the API call to remove a peer from the gateway.
func (d *daemon) gatewayPeerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("address"))
	err := d.gateway.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}
