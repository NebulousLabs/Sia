package api

import (
	"net/http"
)

// MinerStatus contains all of the fields returned when querying the miner's
// status.
type MiningPoolStatus struct {
	BlocksMined      int
	StaleBlocksMined int
	NumConnections   int
}

// minerStatusHandler handles the API call that queries the miner's status.
func (srv *Server) miningpoolStatusHandler(w http.ResponseWriter, req *http.Request) {
	//blocksMined, staleMined := srv.miningpool.BlocksMined()
	mps := MiningPoolStatus{
		BlocksMined:      0,
		StaleBlocksMined: 0,
		NumConnections:   0,
	}
	writeJSON(w, mps)
}
