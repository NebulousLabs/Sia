package main

import (
	"net/http"
)

// Provides the configuration settings for the host.
func (d *daemon) hostConfigHandler(w http.ResponseWriter, req *http.Request) {
	// call d.core.HostInfo to get the config struct.
}

func (d *daemon) hostSetConfigHandler(w http.ResponseWriter, req *http.Request) {
	// marshal a components.HostAnnouncement and call d.core.UpdateHost(theHostAnnouncement)
}
