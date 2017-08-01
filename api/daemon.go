package api

import (
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/julienschmidt/httprouter"
)

func (api *API) memloggingGET(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	if build.MEMLOGGING {
		w.Write([]byte("memlogging enabled"))
	} else {
		w.Write([]byte("memlogging disabled"))
	}
}

func (api *API) memloggingPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	val := strings.ToLower(req.FormValue("set")) // response is case-insensitive
	if val == "true" {
		build.MEMLOGGING = true
		WriteSuccess(w)
		println("api SUCCESS true")
		return
	}
	if val == "false" {
		build.MEMLOGGING = false
		WriteSuccess(w)
		println("api SUCCESS false")
		return
	}
	WriteError(w, Error{"error when calling /daemon/memlogging: expected param 'set' to be 'true' or 'false'"}, http.StatusBadRequest)
}
