package main

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/sia/components"
)

// Provides the configuration settings for the host.
func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	hinfo, err := d.core.HostInfo()
	if err != nil {
		http.Error(w, "Failed to retreive HostInfo: "+err.Error(), 405)
	}

	writeJSON(w, hinfo)
}

func (d *daemon) hostSetConfigHandler(w http.ResponseWriter, req *http.Request) {
	hAnnouncement := components.HostAnnouncement{}

	qsVars := map[string]interface{}{
		"totalstorage": &hAnnouncement.TotalStorage,
		// "minfile":      &hAnnouncement.MinFilesize,
		"maxfilesize":  &hAnnouncement.MaxFilesize,
		"mintolerance": &hAnnouncement.MinTolerance,
		// "minduration":  &hAnnouncement.MinDuration,
		"maxduration": &hAnnouncement.MaxDuration,
		"price":       &hAnnouncement.Price,
		"burn":        &hAnnouncement.Burn,
	}

	for qs := range qsVars {
		_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
		if err != nil {
			http.Error(w, "Malformed "+qs+" "+err.Error(), 400)
			return
		}
	}

	err := d.core.UpdateHost(hAnnouncement)
	if err != nil {
		http.Error(w, "Could not update host:"+err.Error(), 400)
	}

	writeSuccess(w)
}
