package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

// DownloadQueue contains the renter's download queue.
type RenterDownloadQueue struct {
	Downloads []modules.DownloadInfo `json:"downloads"`
}

// RenterFiles lists the files known to the renter.
type RenterFiles struct {
	Files []modules.FileInfo `json:"files"`
}

// RenterLoad lists files that were loaded into the renter.
type RenterLoad struct {
	FilesAdded []string `json:"filesadded"`
}

// RenterShareASCII contains an ASCII-encoded .sia file.
type RenterShareASCII struct {
	File string `json:"file"`
}

// ActiveHosts lists active hosts on the network.
type ActiveHosts struct {
	Hosts []modules.HostSettings `json:"hosts"`
}

// renterDownloadsHandler handles the API call to request the download queue.
func (srv *Server) renterDownloadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, RenterDownloadQueue{
		Downloads: srv.renter.DownloadQueue(),
	})
}

// renterLoadHandler handles the API call to load a '.sia' file.
func (srv *Server) renterLoadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := srv.renter.LoadSharedFiles(req.FormValue("filename"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterLoad{FilesAdded: files})
}

// renterLoadAsciiHandler handles the API call to load a '.sia' file
// in ASCII form.
func (srv *Server) renterLoadAsciiHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := srv.renter.LoadSharedFilesAscii(req.FormValue("file"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterLoad{FilesAdded: files})
}

// renterRenameHandler handles the API call to rename a file entry in the
// renter.
func (srv *Server) renterRenameHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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

// renterFilesHandler handles the API call to list all of the files.
func (srv *Server) renterFilesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, RenterFiles{
		Files: srv.renter.FileList(),
	})
}

// renterDeleteHander handles the API call to delete a file entry from the
// renter.
func (srv *Server) renterDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.DeleteFile(strings.TrimPrefix(ps.ByName("path"), "/"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterDownloadHandler handles the API call to download a file.
func (srv *Server) renterDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.Download(strings.TrimPrefix(ps.ByName("path"), "/"), req.FormValue("destination"))
	if err != nil {
		writeError(w, "Download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterShareHandler handles the API call to create a '.sia' file that
// shares a set of file.
func (srv *Server) renterShareHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.ShareFiles(strings.Split(req.FormValue("path"), ","), req.FormValue("destination"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterShareAsciiHandler handles the API call to return a '.sia' file
// in ascii form.
func (srv *Server) renterShareAsciiHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ascii, err := srv.renter.ShareFilesAscii(strings.Split(req.FormValue("path"), ","))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, RenterShareASCII{
		File: ascii,
	})
}

// renterUploadHandler handles the API call to upload a file.
func (srv *Server) renterUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	var duration types.BlockHeight
	if req.FormValue("duration") != "" {
		_, err := fmt.Sscan(req.FormValue("duration"), &duration)
		if err != nil {
			writeError(w, "Couldn't parse duration: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	renew := req.FormValue("renew") == "true"
	err := srv.renter.Upload(modules.FileUploadParams{
		Filename: req.FormValue("source"),
		Nickname: strings.TrimPrefix(ps.ByName("path"), "/"),
		Duration: duration,
		Renew:    renew,
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

// renterHostsActiveHandler handes the API call asking for the list of active
// hosts.
func (srv *Server) renterHostsActiveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, ActiveHosts{
		Hosts: srv.renter.ActiveHosts(),
	})
}

// renterHostsAllHandler handes the API call asking for the list of all hosts.
func (srv *Server) renterHostsAllHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, ActiveHosts{
		Hosts: srv.renter.AllHosts(),
	})
}
