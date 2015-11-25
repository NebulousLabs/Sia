package api

import (
	"io/ioutil"
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
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

// minerHandlerGET handles GET requests to the /miner API endpoint.
func (srv *Server) minerHandlerGET(w http.ResponseWriter, req *http.Request) {
	blocksMined, staleMined := srv.miner.BlocksMined()
	mg := MinerGET{
		BlocksMined:      blocksMined,
		CPUHashrate:      srv.miner.CPUHashrate(),
		CPUMining:        srv.miner.CPUMining(),
		StaleBlocksMined: staleMined,
	}
	writeJSON(w, mg)
}

// minerHandler handles the API call that queries the miner's status.
func (srv *Server) minerHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.minerHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /miner", http.StatusBadRequest)
}

// minerStartHandler handles the API call that starts the miner.
func (srv *Server) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.miner.StartCPUMining()
		writeSuccess(w)
		return
	}
	writeError(w, "unrecognized method when calling /miner/start", http.StatusBadRequest)
}

// minerStopHandler handles the API call to stop the miner.
func (srv *Server) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.miner.StopCPUMining()
		writeSuccess(w)
		return
	}
	writeError(w, "unrecognized method when calling /miner/stop", http.StatusBadRequest)
}

// minerHeaderforworkHandler handles the API call that retrieves a block header
// for work.
func (srv *Server) minerHeaderforworkHandler(w http.ResponseWriter, req *http.Request) {
	bhfw, target, err := srv.miner.HeaderForWork()
	if err != nil {
		writeError(w, "headerforwork operation failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Write(encoding.MarshalAll(target, bhfw))
}

// minerSubmitheaderHandler handles the API call to submit a block header to the
// miner.
func (srv *Server) minerSubmitheaderHandler(w http.ResponseWriter, req *http.Request) {
	var bh types.BlockHeader
	encodedHeader, err := ioutil.ReadAll(req.Body)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = encoding.Unmarshal(encodedHeader, &bh)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = srv.miner.SubmitHeader(bh)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// minerHeaderHandler handles API calls to /miner/header.
func (srv *Server) minerHeaderHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.minerHeaderforworkHandler(w, req)
		return
	} else if req.Method == "POST" {
		srv.minerSubmitheaderHandler(w, req)
		return
	}
}
