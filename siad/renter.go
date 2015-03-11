package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	duration   = 2000 // Duration that hosts will hold onto the file.
	redundancy = 15   // Redundancy of files uploaded to the network.
)

// renterDownloadHandler handles the api call to download a file.
func (d *daemon) renterDownloadHandler(w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(d.downloadDir, req.FormValue("destination"))
	err := d.renter.Download(req.FormValue("nickname"), path)
	if err != nil {
		writeError(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterStatusHandler handles the api call querying the renter's status.
func (d *daemon) renterStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.renter.Info())
}

// renterUploadHandler handles the api call to upload a file using a
// filepath.
func (d *daemon) renterUploadHandler(w http.ResponseWriter, req *http.Request) {
	// open the file
	file, err := os.Open(req.FormValue("source"))
	if err != nil {
		writeError(w, "Couldn't open file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = d.renter.Upload(modules.UploadParams{
		Data:     file,
		Duration: duration,
		Nickname: req.FormValue("nickname"),
		Pieces:   redundancy,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
