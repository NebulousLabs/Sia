package api

import (
	"fmt"
	"net/http"
	"path/filepath"
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

	// AllHosts lists all hosts that the renter is aware of.
	AllHosts struct {
		Hosts []modules.HostDBEntry `json:"hosts"`
	}
)

// renterHandlerGET handles the API call to /renter.
func (api *API) renterHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	renterMetrics, _ := api.renter.Metrics()
	WriteJSON(w, RenterGET{
		Settings:         api.renter.Settings(),
		FinancialMetrics: renterMetrics,
	})
}

// renterHandlerPOST handles the API call to set the Renter's settings.
func (api *API) renterHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// scan values
	funds, ok := scanAmount(req.FormValue("funds"))
	if !ok {
		WriteError(w, Error{"Couldn't parse funds"}, http.StatusBadRequest)
		return
	}
	// var hosts uint64
	// _, err := fmt.Sscan(req.FormValue("hosts"), &hosts)
	// if err != nil {
	// 	WriteError(w, Error{"Couldn't parse hosts: "+err.Error()}, http.StatusBadRequest)
	// 	return
	// }
	var period types.BlockHeight
	_, err := fmt.Sscan(req.FormValue("period"), &period)
	if err != nil {
		WriteError(w, Error{"Couldn't parse period: " + err.Error()}, http.StatusBadRequest)
		return
	}
	// var renewWindow types.BlockHeight
	// _, err = fmt.Sscan(req.FormValue("renewwindow"), &renewWindow)
	// if err != nil {
	// 	WriteError(w, Error{"Couldn't parse renewwindow: "+err.Error()}, http.StatusBadRequest)
	// 	return
	// }

	err = api.renter.SetSettings(modules.RenterSettings{
		Allowance: modules.Allowance{
			Funds:  funds,
			Period: period,

			// TODO: let user specify these
			Hosts:       recommendedHosts,
			RenewWindow: period / 2,
		},
	})
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// renterContractsHandler handles the API call to request the Renter's contracts.
func (api *API) renterContractsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	contracts := []RenterContract{}
	for _, c := range api.renter.Contracts() {
		contracts = append(contracts, RenterContract{
			EndHeight:   c.EndHeight(),
			ID:          c.ID,
			NetAddress:  c.NetAddress,
			RenterFunds: c.RenterFunds(),
			Size:        modules.SectorSize * uint64(len(c.MerkleRoots)),
		})
	}
	WriteJSON(w, RenterContracts{
		Contracts: contracts,
	})
}

// renterDownloadsHandler handles the API call to request the download queue.
func (api *API) renterDownloadsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	WriteJSON(w, RenterDownloadQueue{
		Downloads: api.renter.DownloadQueue(),
	})
}

// renterLoadHandler handles the API call to load a '.sia' file.
func (api *API) renterLoadHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	source := req.FormValue("source")
	if !filepath.IsAbs(source) {
		WriteError(w, Error{"source must be an absolute path"}, http.StatusBadRequest)
		return
	}

	files, err := api.renter.LoadSharedFiles(source)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	WriteJSON(w, RenterLoad{FilesAdded: files})
}

// renterLoadAsciiHandler handles the API call to load a '.sia' file
// in ASCII form.
func (api *API) renterLoadAsciiHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := api.renter.LoadSharedFilesAscii(req.FormValue("asciisia"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	WriteJSON(w, RenterLoad{FilesAdded: files})
}

// renterRenameHandler handles the API call to rename a file entry in the
// renter.
func (api *API) renterRenameHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := api.renter.RenameFile(strings.TrimPrefix(ps.ByName("siapath"), "/"), req.FormValue("newsiapath"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	WriteSuccess(w)
}

// renterFilesHandler handles the API call to list all of the files.
func (api *API) renterFilesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	WriteJSON(w, RenterFiles{
		Files: api.renter.FileList(),
	})
}

// renterDeleteHandler handles the API call to delete a file entry from the
// renter.
func (api *API) renterDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	err := api.renter.DeleteFile(strings.TrimPrefix(ps.ByName("siapath"), "/"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	WriteSuccess(w)
}

// renterDownloadHandler handles the API call to download a file.
func (api *API) renterDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	destination := req.FormValue("destination")
	// Check that the destination path is absolute.
	if !filepath.IsAbs(destination) {
		WriteError(w, Error{"destination must be an absolute path"}, http.StatusBadRequest)
		return
	}

	err := api.renter.Download(strings.TrimPrefix(ps.ByName("siapath"), "/"), destination)
	if err != nil {
		WriteError(w, Error{"Download failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	WriteSuccess(w)
}

// renterShareHandler handles the API call to create a '.sia' file that
// shares a set of file.
func (api *API) renterShareHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	destination := req.FormValue("destination")
	// Check that the destination path is absolute.
	if !filepath.IsAbs(destination) {
		WriteError(w, Error{"destination must be an absolute path"}, http.StatusBadRequest)
		return
	}

	err := api.renter.ShareFiles(strings.Split(req.FormValue("siapaths"), ","), destination)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	WriteSuccess(w)
}

// renterShareAsciiHandler handles the API call to return a '.sia' file
// in ascii form.
func (api *API) renterShareAsciiHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ascii, err := api.renter.ShareFilesAscii(strings.Split(req.FormValue("siapaths"), ","))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, RenterShareASCII{
		ASCIIsia: ascii,
	})
}

// renterUploadHandler handles the API call to upload a file.
func (api *API) renterUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	source := req.FormValue("source")
	if !filepath.IsAbs(source) {
		WriteError(w, Error{"source must be an absolute path"}, http.StatusBadRequest)
		return
	}

	err := api.renter.Upload(modules.FileUploadParams{
		Source:  source,
		SiaPath: strings.TrimPrefix(ps.ByName("siapath"), "/"),
		// let the renter decide these values; eventually they will be configurable
		ErasureCode: nil,
	})
	if err != nil {
		WriteError(w, Error{"Upload failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	WriteSuccess(w)
}

// renterHostsActiveHandler handles the API call asking for the list of active
// hosts.
func (api *API) renterHostsActiveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var numHosts uint64
	hosts := api.renter.ActiveHosts()

	if req.FormValue("numhosts") == "" {
		// Default value for 'numhosts' is all of them.
		numHosts = uint64(len(hosts))
	} else {
		// Parse the value for 'numhosts'.
		_, err := fmt.Sscan(req.FormValue("numhosts"), &numHosts)
		if err != nil {
			WriteError(w, Error{err.Error()}, http.StatusBadRequest)
			return
		}

		// Catch any boundary errors.
		if numHosts > uint64(len(hosts)) {
			numHosts = uint64(len(hosts))
		}
	}

	WriteJSON(w, ActiveHosts{
		Hosts: hosts[:numHosts],
	})
}

// renterHostsAllHandler handles the API call asking for the list of all hosts.
func (api *API) renterHostsAllHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	WriteJSON(w, AllHosts{
		Hosts: api.renter.AllHosts(),
	})
}
