package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

var (
	// recommendedHosts is the number of hosts that the renter will form
	// contracts with if the value is not specified explicitly in the call to
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
		Settings         modules.RenterSettings     `json:"settings"`
		FinancialMetrics modules.ContractorSpending `json:"financialmetrics"`
		CurrentPeriod    types.BlockHeight          `json:"currentperiod"`
	}

	// RenterContract represents a contract formed by the renter.
	RenterContract struct {
		// Amount of contract funds that have been spent on downloads.
		DownloadSpending types.Currency `json:"downloadspending"`
		// Block height that the file contract ends on.
		EndHeight types.BlockHeight `json:"endheight"`
		// Fees paid in order to form the file contract.
		Fees types.Currency `json:"fees"`
		// Public key of the host the contract was formed with.
		HostPublicKey types.SiaPublicKey `json:"hostpublickey"`
		// ID of the file contract.
		ID types.FileContractID `json:"id"`
		// A signed transaction containing the most recent contract revision.
		LastTransaction types.Transaction `json:"lasttransaction"`
		// Address of the host the file contract was formed with.
		NetAddress modules.NetAddress `json:"netaddress"`
		// Remaining funds left for the renter to spend on uploads & downloads.
		RenterFunds types.Currency `json:"renterfunds"`
		// Size of the file contract, which is typically equal to the number of
		// bytes that have been uploaded to the host.
		Size uint64 `json:"size"`
		// Block height that the file contract began on.
		StartHeight types.BlockHeight `json:"startheight"`
		// Amount of contract funds that have been spent on storage.
		StorageSpending types.Currency `json:"storagespending"`
		// DEPRECATED: This is the exact same value as StorageSpending, but it has
		// incorrect capitalization. This was fixed in 1.3.2, but this field is kept
		// to preserve backwards compatibility on clients who depend on the
		// incorrect capitalization. This field will be removed in the future, so
		// clients should switch to the StorageSpending field (above) with the
		// correct lowercase name.
		StorageSpendingDeprecated types.Currency `json:"StorageSpending"`
		// Total cost to the wallet of forming the file contract.
		TotalCost types.Currency `json:"totalcost"`
		// Amount of contract funds that have been spent on uploads.
		UploadSpending types.Currency `json:"uploadspending"`
		// Signals if contract is good for uploading data
		GoodForUpload bool `json:"goodforupload"`
		// Signals if contract is good for a renewal
		GoodForRenew bool `json:"goodforrenew"`
	}

	// RenterContracts contains the renter's contracts.
	RenterContracts struct {
		Contracts []RenterContract `json:"contracts"`
	}

	// RenterDownloadQueue contains the renter's download queue.
	RenterDownloadQueue struct {
		Downloads []DownloadInfo `json:"downloads"`
	}

	// RenterFile lists the file queried.
	RenterFile struct {
		File modules.FileInfo `json:"file"`
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

	// DownloadInfo contains all client-facing information of a file.
	DownloadInfo struct {
		Destination     string `json:"destination"`     // The destination of the download.
		DestinationType string `json:"destinationtype"` // Can be "file", "memory buffer", or "http stream".
		Filesize        uint64 `json:"filesize"`        // DEPRECATED. Same as 'Length'.
		Length          uint64 `json:"length"`          // The length requested for the download.
		Offset          uint64 `json:"offset"`          // The offset within the siafile requested for the download.
		SiaPath         string `json:"siapath"`         // The siapath of the file used for the download.

		Completed            bool      `json:"completed"`            // Whether or not the download has completed.
		EndTime              time.Time `json:"endtime"`              // The time when the download fully completed.
		Error                string    `json:"error"`                // Will be the empty string unless there was an error.
		Received             uint64    `json:"received"`             // Amount of data confirmed and decoded.
		StartTime            time.Time `json:"starttime"`            // The time when the download was started.
		TotalDataTransferred uint64    `json:"totaldatatransferred"` // The total amount of data transferred, including negotiation, overdrive etc.
	}
)

// renterHandlerGET handles the API call to /renter.
func (api *API) renterHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	settings := api.renter.Settings()
	periodStart := api.renter.CurrentPeriod()
	WriteJSON(w, RenterGET{
		Settings:         settings,
		FinancialMetrics: api.renter.PeriodSpending(),
		CurrentPeriod:    periodStart,
	})
}

// renterHandlerPOST handles the API call to set the Renter's settings.
func (api *API) renterHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the existing settings
	settings := api.renter.Settings()

	// Scan the allowance amount. (optional parameter)
	if f := req.FormValue("funds"); f != "" {
		funds, ok := scanAmount(f)
		if !ok {
			WriteError(w, Error{"unable to parse funds"}, http.StatusBadRequest)
			return
		}
		settings.Allowance.Funds = funds
	}
	// Scan the number of hosts to use. (optional parameter)
	if h := req.FormValue("hosts"); h != "" {
		var hosts uint64
		if _, err := fmt.Sscan(h, &hosts); err != nil {
			WriteError(w, Error{"unable to parse hosts: " + err.Error()}, http.StatusBadRequest)
			return
		} else if hosts != 0 && hosts < requiredHosts {
			WriteError(w, Error{fmt.Sprintf("insufficient number of hosts, need at least %v but have %v", recommendedHosts, hosts)}, http.StatusBadRequest)
		} else {
			settings.Allowance.Hosts = hosts
		}
	} else if settings.Allowance.Hosts == 0 {
		// Sane defaults if host haven't been set before.
		settings.Allowance.Hosts = recommendedHosts
	}
	// Scan the period. (optional parameter)
	if p := req.FormValue("period"); p != "" {
		var period types.BlockHeight
		if _, err := fmt.Sscan(p, &period); err != nil {
			WriteError(w, Error{"unable to parse period: " + err.Error()}, http.StatusBadRequest)
			return
		}
		settings.Allowance.Period = types.BlockHeight(period)
	} else if settings.Allowance.Period == 0 {
		WriteError(w, Error{"period needs to be set if it hasn't been set before"}, http.StatusBadRequest)
		return
	}
	// Scan the renew window. (optional parameter)
	if rw := req.FormValue("renewwindow"); rw != "" {
		var renewWindow types.BlockHeight
		if _, err := fmt.Sscan(rw, &renewWindow); err != nil {
			WriteError(w, Error{"unable to parse renewwindow: " + err.Error()}, http.StatusBadRequest)
			return
		} else if renewWindow != 0 && types.BlockHeight(renewWindow) < requiredRenewWindow {
			WriteError(w, Error{fmt.Sprintf("renew window is too small, must be at least %v blocks but have %v blocks", requiredRenewWindow, renewWindow)}, http.StatusBadRequest)
			return
		} else {
			settings.Allowance.RenewWindow = types.BlockHeight(renewWindow)
		}
	} else if settings.Allowance.RenewWindow == 0 {
		// Sane defaults if renew window hasn't been set before.
		settings.Allowance.RenewWindow = settings.Allowance.Period / 2
	}
	// Scan the download speed limit. (optional parameter)
	if d := req.FormValue("maxdownloadspeed"); d != "" {
		var downloadSpeed int64
		if _, err := fmt.Sscan(d, &downloadSpeed); err != nil {
			WriteError(w, Error{"unable to parse downloadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		settings.MaxDownloadSpeed = downloadSpeed
	}
	// Scan the upload speed limit. (optional parameter)
	if u := req.FormValue("maxuploadspeed"); u != "" {
		var uploadSpeed int64
		if _, err := fmt.Sscan(u, &uploadSpeed); err != nil {
			WriteError(w, Error{"unable to parse uploadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		settings.MaxUploadSpeed = uploadSpeed
	}
	// Scan the stream cache size. (optional parameter)
	if dcs := req.FormValue("streamcachesize"); dcs != "" {
		var streamCacheSize uint64
		if _, err := fmt.Sscan(dcs, &streamCacheSize); err != nil {
			WriteError(w, Error{"unable to parse streamcachesize: " + err.Error()}, http.StatusBadRequest)
			return
		}
		settings.StreamCacheSize = streamCacheSize
	}
	// Set the settings in the renter.
	err := api.renter.SetSettings(settings)
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
		var size uint64
		if len(c.Transaction.FileContractRevisions) != 0 {
			size = c.Transaction.FileContractRevisions[0].NewFileSize
		}

		// Fetch host address
		var netAddress modules.NetAddress
		hdbe, exists := api.renter.Host(c.HostPublicKey)
		if exists {
			netAddress = hdbe.NetAddress
		}

		// Fetch utilities for contract
		var goodForUpload bool
		var goodForRenew bool
		if utility, ok := api.renter.ContractUtility(c.HostPublicKey); ok {
			goodForUpload = utility.GoodForUpload
			goodForRenew = utility.GoodForRenew
		}

		contracts = append(contracts, RenterContract{
			DownloadSpending:          c.DownloadSpending,
			EndHeight:                 c.EndHeight,
			Fees:                      c.TxnFee.Add(c.SiafundFee).Add(c.ContractFee),
			GoodForUpload:             goodForUpload,
			GoodForRenew:              goodForRenew,
			HostPublicKey:             c.HostPublicKey,
			ID:                        c.ID,
			LastTransaction:           c.Transaction,
			NetAddress:                netAddress,
			RenterFunds:               c.RenterFunds,
			Size:                      size,
			StartHeight:               c.StartHeight,
			StorageSpending:           c.StorageSpending,
			StorageSpendingDeprecated: c.StorageSpending,
			TotalCost:                 c.TotalCost,
			UploadSpending:            c.UploadSpending,
		})
	}
	WriteJSON(w, RenterContracts{
		Contracts: contracts,
	})
}

// renterDownloadsHandler handles the API call to request the download queue.
func (api *API) renterDownloadsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	var downloads []DownloadInfo
	for _, di := range api.renter.DownloadHistory() {
		downloads = append(downloads, DownloadInfo{
			Destination:     di.Destination,
			DestinationType: di.DestinationType,
			Filesize:        di.Length,
			Length:          di.Length,
			Offset:          di.Offset,
			SiaPath:         di.SiaPath,

			Completed:            di.Completed,
			EndTime:              di.EndTime,
			Error:                di.Error,
			Received:             di.Received,
			StartTime:            di.StartTime,
			TotalDataTransferred: di.TotalDataTransferred,
		})
	}
	// sort the downloads by newest first
	sort.Slice(downloads, func(i, j int) bool { return downloads[i].StartTime.After(downloads[j].StartTime) })
	WriteJSON(w, RenterDownloadQueue{
		Downloads: downloads,
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
func (api *API) renterLoadASCIIHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	files, err := api.renter.LoadSharedFilesASCII(req.FormValue("asciisia"))
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

// renterFileHandler handles the API call to return specific file.
func (api *API) renterFileHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	file, err := api.renter.File(strings.TrimPrefix(ps.ByName("siapath"), "/"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, RenterFile{
		File: file,
	})
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
	params, err := parseDownloadParameters(w, req, ps)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	if params.Async {
		err = api.renter.DownloadAsync(params)
	} else {
		err = api.renter.Download(params)
	}
	if err != nil {
		WriteError(w, Error{"download failed: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	if params.Httpwriter == nil {
		// `httpresp=true` causes writes to w before this line is run, automatically
		// adding `200 Status OK` code to response. Calling this results in a
		// multiple calls to WriteHeaders() errors.
		WriteSuccess(w)
		return
	}
}

// renterDownloadAsyncHandler handles the API call to download a file asynchronously.
func (api *API) renterDownloadAsyncHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	req.ParseForm()
	req.Form.Set("async", "true")
	api.renterDownloadHandler(w, req, ps)
}

// parseDownloadParameters parses the download parameters passed to the
// /renter/download endpoint. Validation of these parameters is done by the
// renter.
func parseDownloadParameters(w http.ResponseWriter, req *http.Request, ps httprouter.Params) (modules.RenterDownloadParameters, error) {
	destination := req.FormValue("destination")

	// The offset and length in bytes.
	offsetparam := req.FormValue("offset")
	lengthparam := req.FormValue("length")

	// Determines whether the response is written to response body.
	httprespparam := req.FormValue("httpresp")

	// Determines whether to return on completion of download or straight away.
	// If httprespparam is present, this parameter is ignored.
	asyncparam := req.FormValue("async")

	// Parse the offset and length parameters.
	var offset, length uint64
	if len(offsetparam) > 0 {
		_, err := fmt.Sscan(offsetparam, &offset)
		if err != nil {
			return modules.RenterDownloadParameters{}, build.ExtendErr("could not decode the offset as uint64: ", err)
		}
	}
	if len(lengthparam) > 0 {
		_, err := fmt.Sscan(lengthparam, &length)
		if err != nil {
			return modules.RenterDownloadParameters{}, build.ExtendErr("could not decode the offset as uint64: ", err)
		}
	}

	// Parse the httpresp parameter.
	httpresp, err := scanBool(httprespparam)
	if err != nil {
		return modules.RenterDownloadParameters{}, build.ExtendErr("httpresp parameter could not be parsed", err)
	}

	// Parse the async parameter.
	async, err := scanBool(asyncparam)
	if err != nil {
		return modules.RenterDownloadParameters{}, build.ExtendErr("async parameter could not be parsed", err)
	}

	siapath := strings.TrimPrefix(ps.ByName("siapath"), "/") // Sia file name.

	dp := modules.RenterDownloadParameters{
		Destination: destination,
		Async:       async,
		Length:      length,
		Offset:      offset,
		SiaPath:     siapath,
	}
	if httpresp {
		dp.Httpwriter = w
	}

	return dp, nil
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
func (api *API) renterShareASCIIHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	ascii, err := api.renter.ShareFilesASCII(strings.Split(req.FormValue("siapaths"), ","))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, RenterShareASCII{
		ASCIIsia: ascii,
	})
}

// renterStreamHandler handles downloads from the /renter/stream endpoint
func (api *API) renterStreamHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	siaPath := strings.TrimPrefix(ps.ByName("siapath"), "/")
	fileName, streamer, err := api.renter.Streamer(siaPath)
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("failed to create download streamer: %v", err)},
			http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, req, fileName, time.Time{}, streamer)
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
