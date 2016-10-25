package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

var (
	// errNoPath is returned when a call fails to provide a nonempty string
	// for the path parameter.
	errNoPath = Error{"path parameter is required"}

	// errStorageFolderNotFound is returned if a call is made looking for a
	// storage folder which does not appear to exist within the storage
	// manager.
	errStorageFolderNotFound = errors.New("storage folder with the provided path could not be found")
)

type (
	// HostGET contains the information that is returned after a GET request to
	// /host - a bunch of information about the status of the host.
	HostGET struct {
		ExternalSettings modules.HostExternalSettings `json:"externalsettings"`
		FinancialMetrics modules.HostFinancialMetrics `json:"financialmetrics"`
		InternalSettings modules.HostInternalSettings `json:"internalsettings"`
		NetworkMetrics   modules.HostNetworkMetrics   `json:"networkmetrics"`
	}

	// StorageGET contains the information that is returned after a GET request
	// to /host/storage - a bunch of information about the status of storage
	// management on the host.
	StorageGET struct {
		Folders []modules.StorageFolderMetadata `json:"folders"`
	}

	XcontractsGET struct {
		Contracts []modules.HostContract `json:"contracts"`
	}
)

// folderIndex determines the index of the storage folder with the provided
// path.
func folderIndex(folderPath string, storageFolders []modules.StorageFolderMetadata) (int, error) {
	for i, sf := range storageFolders {
		if sf.Path == folderPath {
			return i, nil
		}
	}
	return -1, errStorageFolderNotFound
}

// hostHandlerGET handles GET requests to the /host API endpoint, returning key
// information about the host.
func (api *API) hostHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	es := api.host.ExternalSettings()
	fm := api.host.FinancialMetrics()
	is := api.host.InternalSettings()
	nm := api.host.NetworkMetrics()
	hg := HostGET{
		ExternalSettings: es,
		FinancialMetrics: fm,
		InternalSettings: is,
		NetworkMetrics:   nm,
	}
	WriteJSON(w, hg)
}

// hostHandlerPOST handles POST request to the /host API endpoint, which sets
// the internal settings of the host.
func (api *API) hostHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Map each query string to a field in the host settings.
	settings := api.host.InternalSettings()
	qsVars := map[string]interface{}{
		"acceptingcontracts":   &settings.AcceptingContracts,
		"maxduration":          &settings.MaxDuration,
		"maxdownloadbatchsize": &settings.MaxDownloadBatchSize,
		"maxrevisebatchsize":   &settings.MaxReviseBatchSize,
		"netaddress":           &settings.NetAddress,
		"windowsize":           &settings.WindowSize,

		"collateral":       &settings.Collateral,
		"collateralbudget": &settings.CollateralBudget,
		"maxcollateral":    &settings.MaxCollateral,

		"mincontractprice":          &settings.MinContractPrice,
		"mindownloadbandwidthprice": &settings.MinDownloadBandwidthPrice,
		"minstorageprice":           &settings.MinStoragePrice,
		"minuploadbandwidthprice":   &settings.MinUploadBandwidthPrice,
	}

	// Iterate through the query string and replace any fields that have been
	// altered.
	for qs := range qsVars {
		if req.FormValue(qs) != "" { // skip empty values
			_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
			if err != nil {
				WriteError(w, Error{"Malformed " + qs}, http.StatusBadRequest)
				return
			}
		}
	}
	err := api.host.SetInternalSettings(settings)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (api *API) hostAnnounceHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var err error
	if addr := req.FormValue("netaddress"); addr != "" {
		err = api.host.AnnounceAddress(modules.NetAddress(addr))
	} else {
		err = api.host.Announce()
	}
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// hostXcontractsHandler is an experimental/volatile api endpoint. App
// developers are strongly discouraged from using it, as it will change and it
// will break your software.
func (api *API) hostXcontractsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	contracts, err := api.host.Contracts()
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusInternalServerError)
		return
	}
	WriteJSON(w, XcontractsGET{
		Contracts: contracts,
	})
}

// storageHandler returns a bunch of information about storage management on
// the host.
func (api *API) storageHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	WriteJSON(w, StorageGET{
		Folders: api.host.StorageFolders(),
	})
}

// storageFoldersAddHandler adds a storage folder to the storage manager.
func (api *API) storageFoldersAddHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	var folderSize uint64
	_, err := fmt.Sscan(req.FormValue("size"), &folderSize)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	err = api.host.AddStorageFolder(folderPath, folderSize)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// storageFoldersResizeHandler resizes a storage folder in the storage manager.
func (api *API) storageFoldersResizeHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	if folderPath == "" {
		WriteError(w, Error{"path parameter is required"}, http.StatusBadRequest)
		return
	}

	storageFolders := api.host.StorageFolders()
	folderIndex, err := folderIndex(folderPath, storageFolders)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	var newSize uint64
	_, err = fmt.Sscan(req.FormValue("newsize"), &newSize)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	err = api.host.ResizeStorageFolder(folderIndex, newSize)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// storageFoldersRemoveHandler removes a storage folder from the storage
// manager.
func (api *API) storageFoldersRemoveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	if folderPath == "" {
		WriteError(w, Error{"path parameter is required"}, http.StatusBadRequest)
		return
	}

	storageFolders := api.host.StorageFolders()
	folderIndex, err := folderIndex(folderPath, storageFolders)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	force := req.FormValue("force") == "true"
	err = api.host.RemoveStorageFolder(folderIndex, force)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// storageSectorsDeleteHandler handles the call to delete a sector from the
// storage manager.
func (api *API) storageSectorsDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sectorRoot, err := scanHash(ps.ByName("merkleroot"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	err = api.host.DeleteSector(sectorRoot)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}
