package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/julienschmidt/httprouter"
)

type (
	// HostGET contains the information that is returned after a GET request to
	// /host - a bunch of information about the status of the host.
	HostGET struct {
		FinancialMetrics modules.HostFinancialMetrics `json:"financialmetrics"`
		InternalSettings modules.HostInternalSettings `json:"internalsettings"`
		NetworkMetrics   modules.HostNetworkMetrics   `json:"networkmetrics"`
	}
)

// hostHandlerGET handles GET requests to the /host API endpoint, returning key
// information about the host.
func (srv *Server) hostHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	fm := srv.host.FinancialMetrics()
	is := srv.host.InternalSettings()
	nm := srv.host.NetworkMetrics()
	hg := HostGET{
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

		"collateral":            &settings.Collateral,
		"collateralbudget":      &settings.CollateralBudget,
		"maxcollateralfraction": &settings.MaxCollateralFraction,
		"maxcollateral":         &settings.MaxCollateral,

		"downloadlimitgrowth": &settings.DownloadLimitGrowth,
		"downloadlimitcap":    &settings.DownloadLimitCap,
		"downloadspeedlimit":  &settings.DownloadSpeedLimit,
		"uploadlimitgrowth":   &settings.UploadLimitGrowth,
		"uploadlimitcap":      &settings.UploadLimitCap,
		"uploadspeedlimit":    &settings.UploadSpeedLimit,

		"minimumcontractprice":          &settings.MinimumContractPrice,
		"minimumdownloadbandwidthprice": &settings.MinimumDownloadBandwidthPrice,
		"minimumstorageprice":           &settings.MinimumStoragePrice,
		"minimumuploadbandwidthprice":   &settings.MinimumUploadBandwidthPrice,
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
	srv.host.SetInternalSettings(settings)
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
