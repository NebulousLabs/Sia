package main

import (
	"fmt"
	"net/http"
)

// hostAnnounceHandler handles the api call to get the host to announce itself
// to the network.
func (d *daemon) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	err := d.host.Announce(d.network.Address())
	if err != nil {
		writeError(w, "Could not announce host:"+err.Error(), 400)
		return
	}
	writeSuccess(w)
}

// hostConfigHandler handles the api call to set the host configuration.
func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	// load current settings
	config := d.host.Info().HostSettings

	// map each query string to a field in the host announcement object
	qsVars := map[string]interface{}{
		"TotalStorage": &config.TotalStorage,
		"MinFilesize":  &config.MinFilesize,
		"MaxFilesize":  &config.MaxFilesize,
		"MinDuration":  &config.MinDuration,
		"MaxDuration":  &config.MaxDuration,
		"Price":        &config.Price,
		"Collateral":   &config.Collateral,
	}

	for qs := range qsVars {
		// only modify supplied values
		if req.FormValue(qs) != "" {
			_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
			if err != nil {
				writeError(w, "Malformed "+qs, 400)
				return
			}
		}
	}

	d.host.SetSettings(config)
	err := d.host.Announce(d.gateway.Info().Address)
	if err != nil {
		writeError(w, "Could not announce host:"+err.Error(), 400)
		return
	}
	writeSuccess(w)
}

// hostStatusHandler handles the api call that queries the host status.
func (d *daemon) hostStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.host.Info())
}
