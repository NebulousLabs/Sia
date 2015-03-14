package main

import (
	"fmt"
	"net/http"
)

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (d *daemon) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	err := d.host.Announce(d.gateway.Info().Address)
	if err != nil {
		writeError(w, "Could not announce host:"+err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// hostConfigHandler handles the API call to set the host configuration.
func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	// load current settings
	config := d.host.Info().HostSettings

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

	for qs := range qsVars {
		// only modify supplied values
		if req.FormValue(qs) != "" {
			_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
			if err != nil {
				writeError(w, "Malformed "+qs, http.StatusBadRequest)
				return
			}
		}
	}

	d.host.SetSettings(config)
	err := d.host.Announce(d.gateway.Info().Address)
	if err != nil {
		writeError(w, "Could not announce host: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// hostStatusHandler handles the API call that queries the host status.
func (d *daemon) hostStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.host.Info())
}
