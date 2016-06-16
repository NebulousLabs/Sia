package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

type (
	// MinerGET contains the information that is returned after a GET request
	// to /miner.
	MinerGET struct {
		BlocksMined      int  `json:"blocksmined"`
		CPUHashrate      int  `json:"cpuhashrate"`
		CPUMining        bool `json:"cpumining"`
		StaleBlocksMined int  `json:"staleblocksmined"`
	}
)

// minerHandler handles the API call that queries the miner's status.
func (srv *Server) minerHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	blocksMined, staleMined := srv.miner.BlocksMined()
	mg := MinerGET{
		BlocksMined:      blocksMined,
		CPUHashrate:      srv.miner.CPUHashrate(),
		CPUMining:        srv.miner.CPUMining(),
		StaleBlocksMined: staleMined,
	}
	writeJSON(w, mg)
}

// minerStartHandler handles the API call that starts the miner.
func (srv *Server) minerStartHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	srv.miner.StartCPUMining()
	writeSuccess(w)
}

// minerStopHandler handles the API call to stop the miner.
func (srv *Server) minerStopHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	srv.miner.StopCPUMining()
	writeSuccess(w)
}

// minerHeaderHandlerGET handles the API call that retrieves a block header
// for work.
func (srv *Server) minerHeaderHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	bhfw, target, err := srv.miner.HeaderForWork()
	if err != nil {
		writeError(w, APIError{err.Error()}, http.StatusBadRequest)
		return
	}
	w.Write(encoding.MarshalAll(target, bhfw))
}

// minerHeaderHandlerPOST handles the API call to submit a block header to the
// miner.
func (srv *Server) minerHeaderHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var bh types.BlockHeader
	err := encoding.NewDecoder(req.Body).Decode(&bh)
	if err != nil {
		writeError(w, APIError{err.Error()}, http.StatusBadRequest)
		return
	}
	err = srv.miner.SubmitHeader(bh)
	if err != nil {
		writeError(w, APIError{err.Error()}, http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
