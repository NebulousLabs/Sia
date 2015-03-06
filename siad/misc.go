package main

import (
	"net/http"
)

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	err := d.gateway.Synchronize()
	if err != nil {
		writeError(w, "No peers available for syncing", 500)
		return
	}

	writeSuccess(w)
}
