package main

import (
	"fmt"
	"net/http"
)

func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	// load current settings
	config := d.host.Info().HostEntry

	// map each query string to a field in the host announcement object
	qsVars := map[string]interface{}{
		"totalstorage": &config.TotalStorage,
		"minfilesize":  &config.MinFilesize,
		"maxfilesize":  &config.MaxFilesize,
		"minduration":  &config.MinDuration,
		"maxduration":  &config.MaxDuration,
		"price":        &config.Price,
		"burn":         &config.Burn,
	}

	for qs := range qsVars {
		// only modify supplied values
		if req.FormValue(qs) != "" {
			_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
			if err != nil {
				http.Error(w, "Malformed "+qs+" "+err.Error(), 400)
				return
			}
		}
	}

	d.host.SetConfig(config)
	writeSuccess(w)
}

func (d *daemon) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	// err = d.AnnounceHost(1, d.state.Height()+20) // A freeze volume and unlock height.
	// if err != nil {
	// 	http.Error(w, "Could not update host:"+err.Error(), 400)
	// 	return
	// }
	// writeSuccess(w)
}

func (d *daemon) hostStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.host.Info())
}
