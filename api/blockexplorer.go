package api

import (
	"net/http"
)

func (srv *Server) blockexplorerCurrentBlockHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.CurrentBlock())
}
