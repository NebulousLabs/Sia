package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	duration = 2000
	delay    = 20
)

func (d *daemon) fileUploadHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("pieces"))
	if err != nil {
		writeError(w, "Malformed pieces", 400)
		return
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		writeError(w, "Malformed/missing file: "+err.Error(), 400)
		return
	}
	defer file.Close()

	err = d.renter.Upload(modules.UploadParams{
		Data:     file,
		Duration: duration,
		Nickname: req.FormValue("nickname"),
		Pieces:   pieces,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileUploadPathHandler(w http.ResponseWriter, req *http.Request) {
	pieces, err := strconv.Atoi(req.FormValue("pieces"))
	if err != nil {
		writeError(w, "Malformed pieces", 400)
		return
	}

	// open the file
	file, err := os.Open(req.FormValue("filename"))
	if err != nil {
		writeError(w, "Couldn't open file: "+err.Error(), 400)
		return
	}

	err = d.renter.Upload(modules.UploadParams{
		Data:     file,
		Duration: duration,
		Nickname: req.FormValue("nickname"),
		Pieces:   pieces,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileDownloadHandler(w http.ResponseWriter, req *http.Request) {
	path := filepath.Join(d.downloadDir, req.FormValue("filename"))
	err := d.renter.Download(req.FormValue("nickname"), path)
	if err != nil {
		// TODO: if this err is a user error (e.g. bad nickname), return 400 instead
		writeError(w, "Download failed: "+err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) fileStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.renter.Info())
}
