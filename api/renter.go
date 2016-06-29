package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

var (
	// TODO: Replace this function by accepting user input.
	recommendedHosts = func() uint64 {
		if build.Release == "dev" {
			return 2
		}
		if build.Release == "standard" {
			return 24
		}
		if build.Release == "testing" {
			return 1
		}
		panic("unrecognized release constant in api")
	}()
)

type (
	// RenterGET contains various renter metrics.
	RenterGET struct {
		Settings         modules.RenterSettings         `json:"settings"`
		FinancialMetrics modules.RenterFinancialMetrics `json:"financialmetrics"`
	}

	// RenterContract represents a contract formed by the renter.
	RenterContract struct {
		EndHeight   types.BlockHeight    `json:"endheight"`
		ID          types.FileContractID `json:"id"`
		NetAddress  modules.NetAddress   `json:"netaddress"`
		RenterFunds types.Currency       `json:"renterfunds"`
		Size        uint64               `json:"size"`
	}

	// RenterContracts contains the renter's contracts.
	RenterContracts struct {
		Contracts []RenterContract `json:"contracts"`
	}

	// DownloadQueue contains the renter's download queue.
	RenterDownloadQueue struct {
		Downloads []modules.DownloadInfo `json:"downloads"`
	}

	// RenterFiles lists the files known to the renter.
	RenterFiles struct {
		Files []modules.FileInfo `json:"files"`
	}

	// RenterLoad lists files that were loaded into the renter.
	RenterLoad struct {
		FilesAdded []string `json:"filesadded"`
	}

	// RenterShareASCII contains an ASCII-encoded .sia file.
	RenterShareASCII struct {
		ASCIIsia string `json:"asciisia"`
	}

	// ActiveHosts lists active hosts on the network.
	ActiveHosts struct {
		Hosts []modules.HostDBEntry `json:"hosts"`
	}
)

// renterHandlerGET handles the API call to /renter.
func (srv *Server) renterHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, RenterGET{
		Settings:         srv.renter.Settings(),
		FinancialMetrics: srv.renter.FinancialMetrics(),
	})
}

// renterHandlerPOST handles the API call to set the Renter's settings.
func (srv *Server) renterHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// scan values
	funds, ok := scanAmount(req.FormValue("funds"))
	if !ok {
		writeError(w, Error{"Couldn't parse funds"}, http.StatusBadRequest)
		return
	}
	// var hosts uint64
	// _, err := fmt.Sscan(req.FormValue("hosts"), &hosts)
	// if err != nil {
	// 	writeError(w, Error{"Couldn't parse hosts: "+err.Error()}, http.StatusBadRequest)
	// 	return
	// }
	var period types.BlockHeight
	_, err := fmt.Sscan(req.FormValue("period"), &period)
	if err != nil {
		writeError(w, Error{"Couldn't parse period: " + err.Error()}, http.StatusBadRequest)
		return
	}
	// var renewWindow types.BlockHeight
	// _, err = fmt.Sscan(req.FormValue("renewwindow"), &renewWindow)
	// if err != nil {
	// 	writeError(w, Error{"Couldn't parse renewwindow: "+err.Error()}, http.StatusBadRequest)
	// 	return
	// }

	err = srv.renter.SetSettings(modules.RenterSettings{
		Allowance: modules.Allowance{
			Funds:  funds,
			Period: period,

			// TODO: let user specify these
			Hosts:       recommendedHosts,
			RenewWindow: period / 2,
		},
	})
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// renterContractsHandler handles the API call to request the Renter's contracts.
func (srv *Server) renterContractsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	contracts := []RenterContract{}
	for _, c := range srv.renter.Contracts() {
		contracts = append(contracts, RenterContract{
			EndHeight:   c.EndHeight(),
			ID:          c.ID,
			NetAddress:  c.NetAddress,
			RenterFunds: c.RenterFunds(),
			Size:        modules.SectorSize * uint64(len(c.MerkleRoots)),
		})
	}
	writeJSON(w, RenterContracts{
		Contracts: contracts,
	})
}

// renterDownloadsHandler handles the API call to request the download queue.
func (srv *Server) renterDownloadsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writeJSON(w, RenterDownloadQueue{
		Downloads: srv.renter.DownloadQueue(),
	})
}

// renterLoadHandler handles the API call to load a '.sia' file.
func (srv *Server) renterLoadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := srv.renter.LoadSharedFiles(req.FormValue("source"))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterLoad{FilesAdded: files})
}

// renterLoadAsciiHandler handles the API call to load a '.sia' file
// in ASCII form.
func (srv *Server) renterLoadAsciiHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := srv.renter.LoadSharedFilesAscii(req.FormValue("asciisia"))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	writeJSON(w, RenterLoad{FilesAdded: files})
}

// renterRenameHandler handles the API call to rename a file entry in the
// renter.
func (srv *Server) renterRenameHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.RenameFile(strings.TrimPrefix(ps.ByName("siapath"), "/"), req.FormValue("newsiapath"))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	writeSuccess(w)
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
	err := srv.renter.DeleteFile(strings.TrimPrefix(ps.ByName("siapath"), "/"))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterDownloadHandler handles the API call to download a file.
func (srv *Server) renterDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.Download(strings.TrimPrefix(ps.ByName("siapath"), "/"), req.FormValue("destination"))
	if err != nil {
		writeError(w, Error{"Download failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterShareHandler handles the API call to create a '.sia' file that
// shares a set of file.
func (srv *Server) renterShareHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.ShareFiles(strings.Split(req.FormValue("siapaths"), ","), req.FormValue("destination"))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	writeSuccess(w)
}

// renterShareAsciiHandler handles the API call to return a '.sia' file
// in ascii form.
func (srv *Server) renterShareAsciiHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ascii, err := srv.renter.ShareFilesAscii(strings.Split(req.FormValue("siapaths"), ","))
	if err != nil {
		writeError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	writeJSON(w, RenterShareASCII{
		ASCIIsia: ascii,
	})
}

// renterUploadHandler handles the API call to upload a file.
func (srv *Server) renterUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := srv.renter.Upload(modules.FileUploadParams{
		Source:  req.FormValue("source"),
		SiaPath: strings.TrimPrefix(ps.ByName("siapath"), "/"),
		// let the renter decide these values; eventually they will be configurable
		ErasureCode: nil,
	})
	if err != nil {
		writeError(w, Error{"Upload failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}

// renterHostsActiveHandler handes the API call asking for the list of active
// hosts.
func (srv *Server) renterHostsActiveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var numHosts uint64
	hosts := srv.renter.ActiveHosts()

	if req.FormValue("numhosts") == "" {
		// Default value for 'numhosts' is all of them.
		numHosts = uint64(len(hosts))
	} else {
		// Parse the value for 'numhosts'.
		_, err := fmt.Sscan(req.FormValue("numhosts"), &numHosts)
		if err != nil {
			writeError(w, Error{err.Error()}, http.StatusBadRequest)
			return
		}

		// Catch any boundary errors.
		if numHosts > uint64(len(hosts)) {
			numHosts = uint64(len(hosts))
		}
	}

	writeJSON(w, ActiveHosts{
		Hosts: hosts[:numHosts],
	})
}

// renterHostsAllHandler handes the API call asking for the list of all hosts.
func (srv *Server) renterHostsAllHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, ActiveHosts{
		Hosts: srv.renter.AllHosts(),
	})
}
