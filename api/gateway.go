package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

// TODO: GatewayInfo is not the right name for this struct.
type GatewayInfo struct {
	NetAddress modules.NetAddress `json:"netaddress"`
	Peers      []modules.Peer     `json:"peers"`
}

// gatewayHandler handles the API call asking for the gatway status.
func (srv *Server) gatewayHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	peers := srv.gateway.Peers()
	// nil slices are marshalled as 'null' in JSON, whereas 0-length slices are
	// marshalled as '[]'. The latter is preferred, indicating that the value
	// exists but contains no elements.
	if peers == nil {
		peers = make([]modules.Peer, 0)
	}
	writeJSON(w, GatewayInfo{srv.gateway.Address(), peers})
}

// gatewayConnectHandler handles the API call to add a peer to the gateway.
func (srv *Server) gatewayConnectHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	addr := modules.NetAddress(ps.ByName("netaddress"))
	err := srv.gateway.Connect(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// gatewayDisconnectHandler handles the API call to remove a peer from the gateway.
func (srv *Server) gatewayDisconnectHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	addr := modules.NetAddress(ps.ByName("netaddress"))
	err := srv.gateway.Disconnect(addr)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}
