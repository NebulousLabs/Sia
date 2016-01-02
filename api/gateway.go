package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

type GatewayInfo struct {
	Address modules.NetAddress
	Peers   []modules.NetAddress
}

// gatewayStatusHandler handles the API call asking for the gatway status.
func (srv *Server) gatewayStatusHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	peers := srv.gateway.Peers()
	if peers == nil {
		peers = make([]modules.NetAddress, 0)
	}
	writeJSON(w, GatewayInfo{srv.gateway.Address(), peers})
}

// gatewayPeersAddHandler handles the API call to add a peer to the gateway.
func (srv *Server) gatewayPeersAddHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	addr := modules.NetAddress(ps.ByName("addr"))
	err := srv.gateway.Connect(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// gatewayPeersRemoveHandler handles the API call to remove a peer from the gateway.
func (srv *Server) gatewayPeersRemoveHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	addr := modules.NetAddress(ps.ByName("addr"))
	err := srv.gateway.Disconnect(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}
