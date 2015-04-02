package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

// gatewayStatusHandler handles the API call asking for the gatway status.
func (srv *Server) gatewayStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.gateway.Info())
}

// gatewaySynchronizeHandler handles the API call asking for the gateway to
// synchronize with other peers.
func (srv *Server) gatewaySynchronizeHandler(w http.ResponseWriter, req *http.Request) {
	peer, err := srv.gateway.RandomPeer()
	if err != nil {
		writeError(w, "No peers available for syncing", http.StatusInternalServerError)
		return
	}
	go srv.gateway.Synchronize(peer)

	writeSuccess(w)
}

// gatewayPeerAddHandler handles the API call to add a peer to the gateway.
func (srv *Server) gatewayPeerAddHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("address"))
	err := srv.gateway.AddPeer(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// gatewayPeerRemoveHandler handles the API call to remove a peer from the gateway.
func (srv *Server) gatewayPeerRemoveHandler(w http.ResponseWriter, req *http.Request) {
	addr := modules.NetAddress(req.FormValue("address"))
	err := srv.gateway.RemovePeer(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}
