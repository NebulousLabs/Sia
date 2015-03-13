package main

import (
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	duration   = 2000 // Duration that hosts will hold onto the file.
	redundancy = 15   // Redundancy of files uploaded to the network.
)

// DownloadInfo is a helper struct for the downloadqueue API call.
type DownloadInfo struct {
	Completed   bool
	Destination string
	Nickname    string
}

// FileInfo is a helper struct for the files API call.
type FileInfo struct {
	Available     bool
	Nickname      string
	Repairing     bool
	TimeRemaining consensus.BlockHeight
}

// renterDownloadHandler handles the API call to download a file.
func (d *daemon) renterDownloadHandler(w http.ResponseWriter, req *http.Request) {
	err := d.renter.Download(req.FormValue("nickname"), req.FormValue("destination"))
	if err != nil {
		writeError(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterDownloadqueueHandler handles the API call to request the download
// queue.
func (d *daemon) renterDownloadqueueHandler(w http.ResponseWriter, req *http.Request) {
	downloads := d.renter.DownloadQueue()
	downloadSet := make([]DownloadInfo, 0, len(downloads))
	for _, dl := range downloads {
		downloadSet = append(downloadSet, DownloadInfo{
			Completed:   dl.Completed(),
			Destination: dl.Destination(),
			Nickname:    dl.Nickname(),
		})
	}

	writeJSON(w, downloadSet)
}

// renterFilesHandler handles the API call to list all of the files.
func (d *daemon) renterFilesHandler(w http.ResponseWriter, req *http.Request) {
	files := d.renter.FileList()
	fileSet := make([]FileInfo, 0, len(files))
	for _, file := range files {
		fileSet = append(fileSet, FileInfo{
			Available:     file.Available(),
			Nickname:      file.Nickname(),
			Repairing:     file.Repairing(),
			TimeRemaining: file.TimeRemaining(),
		})
	}

	writeJSON(w, fileSet)
}

// renterStatusHandler handles the API call querying the renter's status.
func (d *daemon) renterStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.renter.Info())
}

// renterUploadHandler handles the API call to upload a file using a
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
