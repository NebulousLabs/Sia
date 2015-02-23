package main

import (
	"fmt"
	"net/http"
)

func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	// load current settings
	config := d.host.Info().HostSettings

	// map each query string to a field in the host announcement object
	qsVars := map[string]interface{}{
		"totalstorage": &config.TotalStorage,
		"minfilesize":  &config.MinFilesize,
		"maxfilesize":  &config.MaxFilesize,
		"minduration":  &config.MinDuration,
		"maxduration":  &config.MaxDuration,
		"price":        &config.Price,
		"collateral":   &config.Collateral,
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
	writeSuccess(w)
}

func (d *daemon) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	err := d.host.Announce(d.network.Address())
	if err != nil {
		writeError(w, "Could not announce host:"+err.Error(), 400)
		return
	}
	writeSuccess(w)
}

func (d *daemon) hostStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.host.Info())
}
