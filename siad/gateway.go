package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

// gatewayStatusHandler handles the api call asking for the gatway status.
func (d *daemon) gatewayStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.gateway.Info())
}

// gatewaySynchronizeHandler handles the api call asking for the gateway to
// synchronize with other peers.
func (d *daemon) gatewaySynchronizeHandler(w http.ResponseWriter, req *http.Request) {
	err := d.gateway.Synchronize()
	if err != nil {
		writeError(w, "No peers available for syncing", 500)
		return
	}

	writeSuccess(w)
}

// gatewayPeerAddHandler handles the api call to add a peer to the gateway.
func (d *daemon) gatewayPeerAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("Address"))
	err := d.gateway.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}

// gatewayPeerRemoveHandler handles the api call to remove a peer from the gateway.
func (d *daemon) gatewayPeerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := network.Address(req.FormValue("Address"))
	err := d.gateway.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), 400)
		return
	}

	writeSuccess(w)
}
