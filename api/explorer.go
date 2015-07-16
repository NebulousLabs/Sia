package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

// Handles the call to get information about the blockchain. Returns
// several data points such as chain height, the current block, and
// file contract info
func (srv *Server) explorerStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.exp.ExplorerStatus())
}

// Handles the call to get many data points on blocks
func (srv *Server) explorerBlockDataHandler(w http.ResponseWriter, req *http.Request) {
	// Extract the start and end point from the request
	var start, finish types.BlockHeight
	_, err := fmt.Sscan(req.FormValue("start"), &start)
	if err != nil {
		writeError(w, "Malformed or no start height", http.StatusBadRequest)
		return
	}

	// If a range end is not given, assume the range end to be one
	// greater than the range start, returning one block
	_, err = fmt.Sscan(req.FormValue("finish"), &finish)
	if err != nil {
		finish = start + 1
	}

	// Bounds checking is done inside BlockInfo
	blockSummaries, err := srv.exp.BlockInfo(start, finish)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, blockSummaries)
}

// Handles the api call to retrieve data about a specific hash
func (srv *Server) explorerGetHashHandler(w http.ResponseWriter, req *http.Request) {
	// Extract the hash from the request
	var data []byte
	_, err := fmt.Sscanf(req.FormValue("hash"), "%x", &data)
	if err != nil {
		writeError(w, "Malformed or no hash provided: "+err.Error(), http.StatusBadRequest)
		return
	}

	// returnData will be a generic interface. The json encoder
	// should still work though
	returnData, err := srv.exp.GetHashInfo(data)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, returnData)
}
