package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

type (
	// ExplorerGET is the object returned as a response to a GET request to
	// /explorer.
	ExplorerGET struct {
		// General consensus information.
		Height            types.BlockHeight
		Block             types.BlockID
		Target            types.Target
		Difficulty        types.Currency
		MaturityTimestamp types.Timestamp
		Circulation       types.Currency

		// Information about transaction type usage.
		TransactionCount          uint64
		SiacoinInputCount         uint64
		SiacoinOutputCount        uint64
		FileContractCount         uint64
		FileContractRevisionCount uint64
		StorageProofCount         uint64
		SiafundInputCount         uint64
		SiafundOutputCount        uint64
		MinerFeeCount             uint64
		ArbitraryDataCount        uint64
		TransactionSignatureCount uint64

		// Information about file contracts and file contract revisions.
		ActiveContractCount uint64
		ActiveContractCost  types.Currency
		ActiveContractSize  types.Currency
		TotalContractCost   types.Currency
		TotalContractSize   types.Currency
	}

	// ExplorerBlockGET is the object returned as a response to a GET request
	// to /explorer/block.
	ExplorerBlockGET struct {
		ID    types.BlockID
		Block types.Block
		Size  uint64
	}
)

// explorerHandlerGET handles GET requests to /explorer.
func (srv *Server) explorerHandlerGET(w http.ResponseWriter, req *http.Request) {
	stats := srv.explorer.Statistics()
	writeJSON(w, ExplorerGET{
		Height:            stats.Height,
		Block:             stats.Block,
		Target:            stats.Target,
		Difficulty:        stats.Difficulty,
		MaturityTimestamp: stats.MaturityTimestamp,
		Circulation:       stats.Circulation,

		TransactionCount:          stats.TransactionCount,
		SiacoinInputCount:         stats.SiacoinInputCount,
		SiacoinOutputCount:        stats.SiacoinOutputCount,
		FileContractCount:         stats.FileContractCount,
		FileContractRevisionCount: stats.FileContractRevisionCount,
		StorageProofCount:         stats.StorageProofCount,
		SiafundInputCount:         stats.SiafundInputCount,
		SiafundOutputCount:        stats.SiafundOutputCount,
		MinerFeeCount:             stats.MinerFeeCount,
		ArbitraryDataCount:        stats.ArbitraryDataCount,
		TransactionSignatureCount: stats.TransactionSignatureCount,

		ActiveContractCount: stats.ActiveContractCount,
		ActiveContractCost:  stats.ActiveContractCost,
		ActiveContractSize:  stats.ActiveContractSize,
		TotalContractCost:   stats.TotalContractCost,
		TotalContractSize:   stats.TotalContractSize,
	})
}

// explorerHandler handles API calls to /explorer.
func (srv *Server) explorerHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.explorerHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /explorer", http.StatusBadRequest)
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
	blockSummaries, err := srv.explorer.BlockInfo(start, finish)
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
	returnData, err := srv.explorer.GetHashInfo(data)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, returnData)
}
