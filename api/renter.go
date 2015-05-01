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

// renterFileDeleteHander handles the API call to delete a file entry from the
// renter.
func (srv *Server) renterFileDeleteHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.DeleteFile(req.FormValue("nickname"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFileRenameHandler handles the API call to rename a file entry in the
// renter.
func (srv *Server) renterFileRenameHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.RenameFile(req.FormValue("nickname"), req.FormValue("newname"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFileShareLoadHandler handles the API call to load a '.sia' that
// contains filesharing information.
func (srv *Server) renterFileShareLoadHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.LoadSharedFile(req.FormValue("filename"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFileShareLoadAsciiHandler handles the API call to load a '.sia' file
// in ascii form.
func (srv *Server) renterFileShareLoadAsciiHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.LoadSharedFilesAscii(req.FormValue("asciisia"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFileShareSaveHandler handles the API call to create a '.sia' file that
// shares a file.
func (srv *Server) renterFileShareSaveHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.ShareFiles([]string{req.FormValue("nickname")}, req.FormValue("filepath"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFileShareSaveAsciiHandler handles the API call to return a '.sia' file
// in ascii form.
func (srv *Server) renterFileShareSaveAsciiHandler(w http.ResponseWriter, req *http.Request) {
	ascii, err := srv.renter.ShareFilesAscii([]string{req.FormValue("nickname")})
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, struct{ Ascii string }{ascii})
}

// renterStatusHandler handles the API call querying the renter's status.
func (srv *Server) renterStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.renter.Info())
}

// renterUploadHandler handles the API call to upload a file.
func (srv *Server) renterUploadHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.Upload(modules.FileUploadParams{
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
