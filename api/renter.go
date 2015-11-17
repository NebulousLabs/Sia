package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// DownloadInfo is a helper struct for the downloadqueue API call.
type DownloadInfo struct {
	modules.DownloadInfo
}

// FileInfo is a helper struct for the files API call.
type FileInfo struct {
	modules.FileInfo
}

// LoadedFiles lists files that were loaded into the renter.
type RenterFilesLoadResponse struct {
	FilesAdded []string
}

// ActiveHosts is the struct that pads the response to the renter module call
// "ActiveHosts". The padding is used so that the return value can have an
// explicit name, which makes adding or removing fields easier in the future.
type ActiveHosts struct {
	Hosts []modules.HostSettings
}

// renterHostsActiveHandler handes the API call asking for the list of active
// hosts.
func (srv *Server) renterHostsActiveHandler(w http.ResponseWriter, req *http.Request) {
	ah := ActiveHosts{
		Hosts: srv.renter.ActiveHosts(),
	}
	writeJSON(w, ah)
}

// renterHostsAllHandler handes the API call asking for the list of all hosts.
func (srv *Server) renterHostsAllHandler(w http.ResponseWriter, req *http.Request) {
	ah := ActiveHosts{
		Hosts: srv.renter.AllHosts(),
	}
	writeJSON(w, ah)
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
	downloadSet := make([]DownloadInfo, len(downloads))
	for i, dl := range downloads {
		downloadSet[i] = DownloadInfo{dl}
	}

	writeJSON(w, downloadSet)
}

// renterFilesListHandler handles the API call to list all of the files.
func (srv *Server) renterFilesListHandler(w http.ResponseWriter, req *http.Request) {
	files := srv.renter.FileList()
	fileSet := make([]FileInfo, len(files))
	for i, file := range files {
		fileSet[i] = FileInfo{file}
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
	var duration types.BlockHeight
	if req.FormValue("duration") != "" {
		_, err := fmt.Sscan(req.FormValue("duration"), &duration)
		if err != nil {
			writeError(w, "Couldn't parse duration: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	err := srv.renter.Upload(modules.FileUploadParams{
		Filename: req.FormValue("source"),
		Nickname: req.FormValue("nickname"),
		Duration: duration,
		// let the renter decide these values; eventually they will be configurable
		ErasureCode: nil,
		PieceSize:   0,
	})
	if err != nil {
		writeError(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
