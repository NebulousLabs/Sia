package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

// Handles the call to get high level information about the blockchain
func (srv *Server) blockexplorerBlockchainHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.BlockHeight())
}

// Handles the api call to get the current block from the block explorer
func (srv *Server) blockexplorerCurrentBlockHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.CurrentBlock())
}

// Handles the api call to get information about the siacoins in circulation
func (srv *Server) blockexplorerSiacoinsHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.Siacoins())
}

// Handles the api call to get information about the current file contracts
func (srv *Server) blockexplorerFileContractHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.blocke.FileContracts())
}

func (srv *Server) blockexplorerBlockDataHandler(w http.ResponseWriter, req *http.Request) {
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
	blocks, err := srv.blocke.BlockInfo(start, finish)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, blocks)
}
