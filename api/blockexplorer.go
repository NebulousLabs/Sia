package api

import (
	"net/http"
)

// Handles the api call to get the current block from the block explorer
func (srv *Server) blockexplorerCurrentBlockHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.CurrentBlock())
}
