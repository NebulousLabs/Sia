package api

import (
	"fmt"
	"net/http"
)

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (srv *Server) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.host.Announce()
	if err != nil {
		writeError(w, "Could not announce host:"+err.Error(), http.StatusBadRequest)
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
		"totalStorage": &config.TotalStorage,
		"minFilesize":  &config.MinFilesize,
		"maxFilesize":  &config.MaxFilesize,
		"minDuration":  &config.MinDuration,
		"maxDuration":  &config.MaxDuration,
		"windowSize":   &config.WindowSize,
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
