package api

import (
	"net/http"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// DownloadInfo is a helper struct for the downloadqueue API call.
type DownloadInfo struct {
	StartTime   time.Time
	Filesize    uint64
	Received    uint64
	Destination string
	Nickname    string
}

// FileInfo is a helper struct for the files API call.
type FileInfo struct {
	Available      bool
	UploadProgress float32
	Nickname       string
	Filesize       uint64
	TimeRemaining  types.BlockHeight
}

// LoadedFiles lists files that were loaded into the renter.
type RenterFilesLoadResponse struct {
	FilesAdded []string
}

// renterFilesDownloadHandler handles the API call to download a file.
func (srv *Server) renterFilesDownloadHandler(w http.ResponseWriter, req *http.Request) {
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
			StartTime:   dl.StartTime(),
			Filesize:    dl.Filesize(),
			Received:    dl.Received(),
			Destination: dl.Destination(),
			Nickname:    dl.Nickname(),
		})
	}

	writeJSON(w, downloadSet)
}

// renterFilesListHandler handles the API call to list all of the files.
func (srv *Server) renterFilesListHandler(w http.ResponseWriter, req *http.Request) {
	files := srv.renter.FileList()
	fileSet := make([]FileInfo, 0, len(files))
	for _, file := range files {
		fileSet = append(fileSet, FileInfo{
			Available:      file.Available(),
			UploadProgress: file.UploadProgress(),
			Nickname:       file.Nickname(),
			Filesize:       file.Filesize(),
			TimeRemaining:  file.Expiration() - types.BlockHeight(srv.blockchainHeight),
		})
	}

	writeJSON(w, fileSet)
}

// renterFilesDeleteHander handles the API call to delete a file entry from the
// renter.
func (srv *Server) renterFilesDeleteHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.DeleteFile(req.FormValue("nickname"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFilesRenameHandler handles the API call to rename a file entry in the
// renter.
func (srv *Server) renterFilesRenameHandler(w http.ResponseWriter, req *http.Request) {
	writeError(w, "renaming temporarily disabled", http.StatusBadRequest)

	/*
		err := srv.renter.RenameFile(req.FormValue("nickname"), req.FormValue("newname"))
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeSuccess(w)
	*/
}

// renterFilesLoadHandler handles the API call to load a '.sia' file.
func (srv *Server) renterFilesLoadHandler(w http.ResponseWriter, req *http.Request) {
	files, err := srv.renter.LoadSharedFiles(req.FormValue("filename"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterFilesLoadResponse{FilesAdded: files})
}

// renterFilesLoadAsciiHandler handles the API call to load a '.sia' file
// in ASCII form.
func (srv *Server) renterFilesLoadAsciiHandler(w http.ResponseWriter, req *http.Request) {
	files, err := srv.renter.LoadSharedFilesAscii(req.FormValue("file"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterFilesLoadResponse{FilesAdded: files})
}

// renterFilesShareHandler handles the API call to create a '.sia' file that
// shares a file.
// TODO: allow sharing of multiple files.
func (srv *Server) renterFilesShareHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.ShareFiles([]string{req.FormValue("nickname")}, req.FormValue("filepath"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterFilesShareAsciiHandler handles the API call to return a '.sia' file
// in ascii form.
func (srv *Server) renterFilesShareAsciiHandler(w http.ResponseWriter, req *http.Request) {
	ascii, err := srv.renter.ShareFilesAscii([]string{req.FormValue("nickname")})
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, struct{ File string }{ascii})
}

// renterStatusHandler handles the API call querying the renter's status.
func (srv *Server) renterStatusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, srv.renter.Info())
}

// renterFilesUploadHandler handles the API call to upload a file.
func (srv *Server) renterFilesUploadHandler(w http.ResponseWriter, req *http.Request) {
	err := srv.renter.Upload(modules.FileUploadParams{
		Filename: req.FormValue("source"),
		Nickname: req.FormValue("nickname"),
		// let the renter decide these values; eventually they will be configurable
		Duration:    0,
		ErasureCode: nil,
		PieceSize:   0,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
