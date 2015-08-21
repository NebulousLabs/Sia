package api

import (
	"io/ioutil"
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// MinerStatus contains all of the fields returned when querying the miner's
// status.
type MinerStatus struct {
	CPUMining        bool
	CPUHashrate      int // hashes per second
	BlocksMined      int
	StaleBlocksMined int
}

// minerBlockforworkHandler handles the API call that retrieves a block for
// work.
func (srv *Server) minerBlockforworkHandler(w http.ResponseWriter, req *http.Request) {
	bfw, _, target, err := srv.miner.BlockForWork()
	if err != nil {
		writeError(w, "call to /miner/blockforwork failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Write(encoding.MarshalAll(target, bfw.Header(), bfw))
}

// minerHeaderforworkHandler handles the API call that retrieves a block header
// for work.
func (srv *Server) minerHeaderforworkHandler(w http.ResponseWriter, req *http.Request) {
	bhfw, target, err := srv.miner.HeaderForWork()
	if err != nil {
		writeError(w, "call to /miner/headerforwork failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Write(encoding.MarshalAll(target, bhfw))
}

// minerStartHandler handles the API call that starts the miner.
func (srv *Server) minerStartHandler(w http.ResponseWriter, req *http.Request) {
	srv.miner.StartCPUMining()
	writeSuccess(w)
}

// minerStatusHandler handles the API call that queries the miner's status.
func (srv *Server) minerStatusHandler(w http.ResponseWriter, req *http.Request) {
	blocksMined, staleMined := srv.miner.BlocksMined()
	ms := MinerStatus{
		CPUMining:        srv.miner.CPUMining(),
		CPUHashrate:      srv.miner.CPUHashrate(),
		BlocksMined:      blocksMined,
		StaleBlocksMined: staleMined,
	}
	writeJSON(w, ms)
}

// minerStopHandler handles the API call to stop the miner.
func (srv *Server) minerStopHandler(w http.ResponseWriter, req *http.Request) {
	srv.miner.StopCPUMining()
	writeSuccess(w)
}

// minerSubmitblockHandler handles the API call to submit a block to the miner.
func (srv *Server) minerSubmitblockHandler(w http.ResponseWriter, req *http.Request) {
	var b types.Block
	encodedBlock, err := ioutil.ReadAll(req.Body)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = encoding.Unmarshal(encodedBlock, &b)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = srv.miner.SubmitBlock(b)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
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

// minerConnectToPoolHandler handles the API call to connect to a mining pool
func (srv *Server) minerPoolConnectHandler(w http.ResponseWriter, req *http.Request) {
	var ip string
	// TODO: actually read ip from req.Body
	/*encodedIP, err := ioutil.ReadAll(req.Body)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = encoding.Unmarshal(encodedIP, &ip)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}*/
	ip = "localhost:9983" // Miner is temporarily hard-coded to localhost:9983
	err := srv.miner.PoolConnect(ip)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// minerPoolHeaderForWorkHandler handles the API call to request a header
// from the miner for pool ming
func (srv *Server) minerPoolHeaderForWorkHandler(w http.ResponseWriter, req *http.Request) {
	bhfw, target, err := srv.miner.PoolHeaderForWork()
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Write(encoding.MarshalAll(target, bhfw))
}

// minerSubmitPoolHeaderHandler handles the API call to submit a header to
// the miner
func (srv *Server) minerPoolSubmitHeaderHandler(w http.ResponseWriter, req *http.Request) {
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
	err = srv.miner.PoolSubmitHeader(bh)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
