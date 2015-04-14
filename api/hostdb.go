package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

// ActiveHosts is the struct that pads the response to the hostdb module call
// "ActiveHosts". The padding is used so that the return value can have an
// explicit name, which makes adding or removing fields easier in the future.
type ActiveHosts struct {
	Hosts []modules.HostSettings
}

// hostdbHostsActiveHandler handes the API call asking for the list of active
// hosts.
func (srv *Server) hostdbHostsActiveHandler(w http.ResponseWriter, req *http.Request) {
	ah := ActiveHosts{
		Hosts: srv.hostdb.ActiveHosts(),
	}
	writeJSON(w, ah)
}

// hostdbHostsAllHandler handes the API call asking for the list of all hosts.
func (srv *Server) hostdbHostsAllHandler(w http.ResponseWriter, req *http.Request) {
	ah := ActiveHosts{
		Hosts: srv.hostdb.AllHosts(),
	}
	writeJSON(w, ah)
}
