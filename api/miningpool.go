package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

type (
	// PoolGET contains the information that is returned after a GET request
	// to /pool.
	PoolGET struct {
		PoolRunning  bool `json:"poolrunning"`
		BlocksMined  int  `json:"blocksmined"`
		PoolHashrate int  `json:"cpuhashrate"`
	}
)

// poolHandler handles the API call that queries the pool's status.
func (api *API) poolHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	pg := PoolGET{
		PoolRunning:  api.pool.GetRunning(),
		BlocksMined:  0,
		PoolHashrate: 0,
	}
	WriteJSON(w, pg)
}

// poolStartHandler handles the API call that starts the pool.
func (api *API) poolStartHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.pool.StartPool()
	WriteSuccess(w)
}

// poolStopHandler handles the API call to stop the pool.
func (api *API) poolStopHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	api.pool.StopPool()
	WriteSuccess(w)
}
