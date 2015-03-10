package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	duration = 2000 // Duration that hosts will hold onto the file.
)

// renterDownloadHandler handles the api call to download a file.
func (d *daemon) renterDownloadHandler(w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(d.downloadDir, req.FormValue("Destination"))
	err := d.renter.Download(req.FormValue("Nickname"), path)
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
// datastream.
func (d *daemon) renterUploadHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("Pieces"))
	if err != nil {
		writeError(w, "Malformed pieces", http.StatusBadRequest)
		return
	}

	file, _, err := req.FormFile("Source")
	if err != nil {
		writeError(w, "Malformed/missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	err = d.renter.Upload(modules.UploadParams{
		Data:     file,
		Duration: duration,
		Nickname: req.FormValue("Nickname"),
		Pieces:   pieces,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterUploadPathHandler handles the api call to upload a file using a
// filepath.
func (d *daemon) renterUploadPathHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("Pieces"))
	if err != nil {
		writeError(w, "Malformed pieces", http.StatusBadRequest)
		return
	}

	// open the file
	file, err := os.Open(req.FormValue("Source"))
	if err != nil {
		writeError(w, "Couldn't open file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = d.renter.Upload(modules.UploadParams{
		Data:     file,
		Duration: duration,
		Nickname: req.FormValue("Nickname"),
		Pieces:   pieces,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
