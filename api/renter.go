package api

// TODO: When setting renter settings, leave empty values unchanged instead of
// zeroing them out.

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

var (
	// recommendedHosts is the number of hosts that the renter will form
	// contracts with if the value is not specified explicity in the call to
	// SetSettings.
	recommendedHosts = build.Select(build.Var{
		Standard: uint64(50),
		Dev:      uint64(2),
		Testing:  uint64(1),
	}).(uint64)

	// requiredHosts specifies the minimum number of hosts that must be set in
	// the renter settings for the renter settings to be valid. This minimum is
	// there to prevent users from shooting themselves in the foot.
	requiredHosts = build.Select(build.Var{
		Standard: uint64(20),
		Dev:      uint64(1),
		Testing:  uint64(1),
	}).(uint64)

	// requiredParityPieces specifies the minimum number of parity pieces that
	// must be used when uploading a file. This minimum exists to prevent users
	// from shooting themselves in the foot.
	requiredParityPieces = build.Select(build.Var{
		Standard: int(12),
		Dev:      int(0),
		Testing:  int(0),
	}).(int)

	// requiredRedundancy specifies the minimum redundancy that will be
	// accepted by the renter when uploading a file. This minimum exists to
	// prevent users from shooting themselves in the foot.
	requiredRedundancy = build.Select(build.Var{
		Standard: float64(2),
		Dev:      float64(1),
		Testing:  float64(1),
	}).(float64)

	// requiredRenewWindow establishes the minimum allowed renew window for the
	// renter settings. This minimum is here to prevent users from shooting
	// themselves in the foot.
	requiredRenewWindow = build.Select(build.Var{
		Standard: types.BlockHeight(288),
		Dev:      types.BlockHeight(1),
		Testing:  types.BlockHeight(1),
	}).(types.BlockHeight)
)

type (
	// RenterGET contains various renter metrics.
	RenterGET struct {
		Settings         modules.RenterSettings `json:"settings"`
		FinancialMetrics RenterFinancialMetrics `json:"financialmetrics"`
		CurrentPeriod    types.BlockHeight      `json:"currentperiod"`
	}

	// RenterFinancialMetrics contains metrics about how much the Renter has
	// spent on storage, uploads, and downloads.
	RenterFinancialMetrics struct {
		// Amount of money in the allowance spent on file contracts including
		// fees.
		ContractSpending types.Currency `json:"contractspending"`

		DownloadSpending types.Currency `json:"downloadspending"`
		StorageSpending  types.Currency `json:"storagespending"`
		UploadSpending   types.Currency `json:"uploadspending"`

		// Amount of money in the allowance that has not been spent.
		Unspent types.Currency `json:"unspent"`
	}

	// RenterContract represents a contract formed by the renter.
	RenterContract struct {
		EndHeight       types.BlockHeight    `json:"endheight"`
		ID              types.FileContractID `json:"id"`
		LastTransaction types.Transaction    `json:"lasttransaction"`
		NetAddress      modules.NetAddress   `json:"netaddress"`
		RenterFunds     types.Currency       `json:"renterfunds"`
		Size            uint64               `json:"size"`
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

	// RenterPricesGET lists the data that is returned when a GET call is made
	// to /renter/prices.
	RenterPricesGET struct {
		modules.RenterPriceEstimation
	}

	// RenterShareASCII contains an ASCII-encoded .sia file.
	RenterShareASCII struct {
		ASCIIsia string `json:"asciisia"`
	}
)

// renterHandlerGET handles the API call to /renter.
func (api *API) renterHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	settings := api.renter.Settings()
	periodStart := api.renter.CurrentPeriod()
	// calculate financial metrics from contracts. We use the special
	// AllContracts method to include contracts that are offline.
	var fm RenterFinancialMetrics
	fm.Unspent = settings.Allowance.Funds
	contracts := api.renter.(interface {
		AllContracts() []modules.RenterContract
	}).AllContracts()
	for _, c := range contracts {
		if c.StartHeight < periodStart {
			continue
		}
		fm.ContractSpending = fm.ContractSpending.Add(c.TotalCost)
		fm.DownloadSpending = fm.DownloadSpending.Add(c.DownloadSpending)
		fm.UploadSpending = fm.UploadSpending.Add(c.UploadSpending)
		fm.StorageSpending = fm.StorageSpending.Add(c.StorageSpending)
		// total unspent is:
		//    allowance - (cost to form contracts) + (money left in contracts)
		if fm.Unspent.Add(c.RenterFunds()).Cmp(c.TotalCost) > 0 {
			fm.Unspent = fm.Unspent.Add(c.RenterFunds()).Sub(c.TotalCost)
		}
	}

	WriteJSON(w, RenterGET{
		Settings:         settings,
		FinancialMetrics: fm,
		CurrentPeriod:    periodStart,
	})
}

// renterHandlerPOST handles the API call to set the Renter's settings.
func (api *API) renterHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Scan the allowance amount.
	funds, ok := scanAmount(req.FormValue("funds"))
	if !ok {
		WriteError(w, Error{"unable to parse funds"}, http.StatusBadRequest)
		return
	}

	// Scan the number of hosts to use. (optional parameter)
	var hosts uint64
	if req.FormValue("hosts") != "" {
		_, err := fmt.Sscan(req.FormValue("hosts"), &hosts)
		if err != nil {
			WriteError(w, Error{"unable to parse hosts: " + err.Error()}, http.StatusBadRequest)
			return
		}
		if hosts != 0 && hosts < requiredHosts {
			WriteError(w, Error{fmt.Sprintf("insufficient number of hosts, need at least %v but have %v", recommendedHosts, hosts)}, http.StatusBadRequest)
			return
		}
	} else {
		hosts = recommendedHosts
	}

	// Scan the period.
	var period types.BlockHeight
	_, err := fmt.Sscan(req.FormValue("period"), &period)
	if err != nil {
		WriteError(w, Error{"unable to parse period: " + err.Error()}, http.StatusBadRequest)
		return
	}

	// Scan the renew window. (optional parameter)
	var renewWindow types.BlockHeight
	if req.FormValue("renewwindow") != "" {
		_, err = fmt.Sscan(req.FormValue("renewwindow"), &renewWindow)
		if err != nil {
			WriteError(w, Error{"unable to parse renewwindow: " + err.Error()}, http.StatusBadRequest)
			return
		}
		if renewWindow != 0 && renewWindow < requiredRenewWindow {
			WriteError(w, Error{fmt.Sprintf("renew window is too small, must be at least %v blocks but have %v blocks", requiredRenewWindow, renewWindow)}, http.StatusBadRequest)
			return
		}
	} else {
		renewWindow = period / 2
	}

	// Set the settings in the renter.
	err = api.renter.SetSettings(modules.RenterSettings{
		Allowance: modules.Allowance{
			Funds:       funds,
			Hosts:       hosts,
			Period:      period,
			RenewWindow: renewWindow,
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
			EndHeight:       c.EndHeight(),
			ID:              c.ID,
			NetAddress:      c.NetAddress,
			LastTransaction: c.LastRevisionTxn,
			RenterFunds:     c.RenterFunds(),
			Size:            c.LastRevision.NewFileSize,
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

// renterPricesHandler reports the expected costs of various actions given the
// renter settings and the set of available hosts.
func (api *API) renterPricesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	WriteJSON(w, RenterPricesGET{
		RenterPriceEstimation: api.renter.PriceEstimation(),
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
		WriteError(w, Error{"download failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}

	WriteSuccess(w)
}

// renterDownloadAsyncHandler handles the API call to download a file asynchronously.
func (api *API) renterDownloadAsyncHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	destination := req.FormValue("destination")
	// Check that the destination path is absolute.
	if !filepath.IsAbs(destination) {
		WriteError(w, Error{"destination must be an absolute path"}, http.StatusBadRequest)
		return
	}

	go api.renter.Download(strings.TrimPrefix(ps.ByName("siapath"), "/"), destination)

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

	// Check whether the erasure coding parameters have been supplied.
	var ec modules.ErasureCoder
	if req.FormValue("datapieces") != "" || req.FormValue("paritypieces") != "" {
		// Check that both values have been supplied.
		if req.FormValue("datapieces") == "" || req.FormValue("paritypieces") == "" {
			WriteError(w, Error{"must provide both the datapieces paramaeter and the paritypieces parameter if specifying erasure coding parameters"}, http.StatusBadRequest)
			return
		}

		// Parse the erasure coding parameters.
		var dataPieces, parityPieces int
		_, err := fmt.Sscan(req.FormValue("datapieces"), &dataPieces)
		if err != nil {
			WriteError(w, Error{"unable to read parameter 'datapieces': " + err.Error()}, http.StatusBadRequest)
			return
		}
		_, err = fmt.Sscan(req.FormValue("paritypieces"), &parityPieces)
		if err != nil {
			WriteError(w, Error{"unable to read parameter 'paritypieces': " + err.Error()}, http.StatusBadRequest)
			return
		}

		// Verify that sane values for parityPieces and redundancy are being
		// supplied.
		if parityPieces < requiredParityPieces {
			WriteError(w, Error{fmt.Sprintf("a minimum of %v parity pieces is required, but %v parity pieces requested", parityPieces, requiredParityPieces)}, http.StatusBadRequest)
			return
		}
		redundancy := float64(dataPieces+parityPieces) / float64(dataPieces)
		if float64(dataPieces+parityPieces)/float64(dataPieces) < requiredRedundancy {
			WriteError(w, Error{fmt.Sprintf("a redundancy of %.2f is required, but redundancy of %.2f supplied", redundancy, requiredRedundancy)}, http.StatusBadRequest)
			return
		}

		// Create the erasure coder.
		ec, err = renter.NewRSCode(dataPieces, parityPieces)
		if err != nil {
			WriteError(w, Error{"unable to encode file using the provided parameters: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}

	// Call the renter to upload the file.
	err := api.renter.Upload(modules.FileUploadParams{
		Source:      source,
		SiaPath:     strings.TrimPrefix(ps.ByName("siapath"), "/"),
		ErasureCode: ec,
	})
	if err != nil {
		WriteError(w, Error{"upload failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	WriteSuccess(w)
}
