package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

var (
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
	// to /storage - a bunch of information about the status of storage
	// management on the host.
	StorageGET struct {
		StorageFolderMetadata []modules.StorageFolderMetadata
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
func (srv *Server) hostHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	es := srv.host.ExternalSettings()
	fm := srv.host.FinancialMetrics()
	is := srv.host.InternalSettings()
	nm := srv.host.NetworkMetrics()
	hg := HostGET{
		ExternalSettings: es,
		FinancialMetrics: fm,
		InternalSettings: is,
		NetworkMetrics:   nm,
	}
	writeJSON(w, hg)
}

// hostHandlerPOST handles POST request to the /host API endpoint, which sets
// the internal settings of the host.
func (srv *Server) hostHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Map each query string to a field in the host settings.
	settings := srv.host.InternalSettings()
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
				writeError(w, "Malformed "+qs, http.StatusBadRequest)
				return
			}
		}
	}
	err := srv.host.SetInternalSettings(settings)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (srv *Server) hostAnnounceHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var err error
	if addr := req.FormValue("netaddress"); addr != "" {
		err = srv.host.AnnounceAddress(modules.NetAddress(addr))
	} else {
		err = srv.host.Announce()
	}
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// storageHandler returns a bunch of information about storage management on
// the host.
func (srv *Server) storageHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sfs := srv.host.StorageFolders()
	sg := StorageGET{
		StorageFolderMetadata: sfs,
	}
	writeJSON(w, sg)
}

// storageFoldersAddHandler adds a storage folder to the storage manager.
func (srv *Server) storageFoldersAddHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	var folderSize uint64
	_, err := fmt.Sscan(req.FormValue("size"), &folderSize)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = srv.host.AddStorageFolder(folderPath, folderSize)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// storageFoldersResizeHandler resizes a storage folder in the storage manager.
func (srv *Server) storageFoldersResizeHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	storageFolders := srv.host.StorageFolders()
	folderIndex, err := folderIndex(folderPath, storageFolders)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var newSize uint64
	_, err = fmt.Sscan(req.FormValue("newsize"), &newSize)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = srv.host.ResizeStorageFolder(folderIndex, newSize)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// storageFoldersRemoveHandler removes a storage folder from the storage
// manager.
func (srv *Server) storageFoldersRemoveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	folderPath := req.FormValue("path")
	storageFolders := srv.host.StorageFolders()
	folderIndex, err := folderIndex(folderPath, storageFolders)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	force := req.FormValue("force") == "true"
	err = srv.host.RemoveStorageFolder(folderIndex, force)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}

// storageSectorsDeleteHandler handles the call to delete a sector from the
// storage manager.
func (srv *Server) storageSectorsDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sectorRoot, err := scanAddress(ps.ByName("merkleroot"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = srv.host.DeleteSector(crypto.Hash(sectorRoot))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
