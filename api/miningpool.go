package api

import (
	"net/http"
)

// MininingPoolStatus contains all of the fields returned when querying the pool's
// status.
type MiningPoolStatus struct {
	BlocksMined      int
	StaleBlocksMined int
	NumConnections   int
}

// miningpoolStatusHandler handles the API call that queries the pool's status.
func (srv *Server) miningpoolStatusHandler(w http.ResponseWriter, req *http.Request) {
	//blocksMined, staleMined := srv.miningpool.BlocksMined()
	mps := MiningPoolStatus{
		BlocksMined:      0,
		StaleBlocksMined: 0,
		NumConnections:   0,
	}
	writeJSON(w, mps)
}
