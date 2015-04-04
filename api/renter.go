package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	duration   = 2000 // Duration that hosts will hold onto the file.
	redundancy = 15   // Redundancy of files uploaded to the network.
)

// DownloadInfo is a helper struct for the downloadqueue API call.
type DownloadInfo struct {
	Complete    bool
	Filesize    uint64
	Received    uint64
	Destination string
	Nickname    string
}

// FileInfo is a helper struct for the files API call.
type FileInfo struct {
	Available     bool
	Nickname      string
	Repairing     bool
	TimeRemaining types.BlockHeight
}

// renterDownloadHandler handles the API call to download a file.
func (srv *Server) renterDownloadHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.Download(req.FormValue("nickname"), req.FormValue("destination"))
	if err != nil {
		writeError(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterDownloadqueueHandler handles the API call to request the download
// queue.
func (srv *Server) renterDownloadqueueHandler(w http.ResponseWriter, req *http.Request) {
	downloads := srv.renter.DownloadQueue()
	downloadSet := make([]DownloadInfo, 0, len(downloads))
	for _, dl := range downloads {
		downloadSet = append(downloadSet, DownloadInfo{
			Complete:    dl.Complete(),
			Filesize:    dl.Filesize(),
			Received:    dl.Received(),
			Destination: dl.Destination(),
			Nickname:    dl.Nickname(),
		})
	}

	writeJSON(w, downloadSet)
}

// renterFilesHandler handles the API call to list all of the files.
func (srv *Server) renterFilesHandler(w http.ResponseWriter, req *http.Request) {
	files := srv.renter.FileList()
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
func (srv *Server) renterStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.renter.Info())
}

// renterUploadHandler handles the API call to upload a file.
func (srv *Server) renterUploadHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.Upload(modules.UploadParams{
		Filename: req.FormValue("source"),
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
