package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (srv *Server) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	// Announce checks that the host is connectible before proceeding. The
	// user can override this check by manually specifying the address.
	var err error
	if addr := req.FormValue("address"); addr != "" {
		err = srv.host.ForceAnnounce(modules.NetAddress(addr))
	} else {
		err = srv.host.Announce()
	}
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// hostConfigureHandler handles the API call to set the host configuration.
func (srv *Server) hostConfigureHandler(w http.ResponseWriter, req *http.Request) {
	// load current settings
	config := srv.host.Info().HostSettings

	// map each query string to a field in the host announcement object
	qsVars := map[string]interface{}{
		"totalstorage": &config.TotalStorage,
		"minfilesize":  &config.MinFilesize,
		"maxfilesize":  &config.MaxFilesize,
		"minduration":  &config.MinDuration,
		"maxduration":  &config.MaxDuration,
		"windowsize":   &config.WindowSize,
		"price":        &config.Price,
		"collateral":   &config.Collateral,
	}

	any := false
	for qs := range qsVars {
		// only modify supplied values
		if req.FormValue(qs) != "" {
			_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
			if err != nil {
				writeError(w, "Malformed "+qs, http.StatusBadRequest)
				return
			}
			any = true
		}
	}
	if !any {
		writeError(w, "No valid configuration fields specified", http.StatusBadRequest)
		return
	}

	srv.host.SetSettings(config)
	writeSuccess(w)
}

// hostStatusHandler handles the API call that queries the host status.
func (srv *Server) hostStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.host.Info())
}
