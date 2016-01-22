package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

type (
	// HostGET contains the information that is returned after a GET request to
	// /host.
	HostGET struct {
		Collateral   types.Currency     `json:"collateral"`
		NetAddress   modules.NetAddress `json:"netaddress"`
		MaxDuration  types.BlockHeight  `json:"maxduration"`
		MinDuration  types.BlockHeight  `json:"minduration"`
		Price        types.Currency     `json:"price"`
		TotalStorage int64              `json:"totalstorage"`
		UnlockHash   types.UnlockHash   `json:"unlockhash"`
		WindowSize   types.BlockHeight  `json:"windowsize"`

		AcceptingContracts bool           `json:"acceptingcontracts"`
		NumContracts       uint64         `json:"numcontracts"`
		LostRevenue        types.Currency `json:"lostrevenue"`
		Revenue            types.Currency `json:"revenue"`
		StorageRemaining   int64          `json:"storageremaining"`
		AnticipatedRevenue types.Currency `json:"anticipatedrevenue"`

		RPCErrorCalls        uint64 `json:"rpcerrorcalls"`
		RPCUnrecognizedCalls uint64 `json:"rpcunrecognizedcalls"`
		RPCDownloadCalls     uint64 `json:"rpcdownloadcalls"`
		RPCRenewCalls        uint64 `json:"rpcrenewcalls"`
		RPCReviseCalls       uint64 `json:"rpcrevisecalls"`
		RPCSettingsCalls     uint64 `json:"rpcsettingscalls"`
		RPCUploadCalls       uint64 `json:"rpcuploadcalls"`
	}
)

// hostHandlerGET handles GET requests to the /host API endpoint.
func (srv *Server) hostHandlerGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	settings := srv.host.Settings()
	anticipatedRevenue, revenue, lostRevenue := srv.host.Revenue()
	rpcCalls := srv.host.RPCMetrics()
	hg := HostGET{
		Collateral:   settings.Collateral,
		NetAddress:   settings.NetAddress,
		MaxDuration:  settings.MaxDuration,
		MinDuration:  settings.MinDuration,
		Price:        settings.Price,
		TotalStorage: settings.TotalStorage,
		UnlockHash:   settings.UnlockHash,
		WindowSize:   settings.WindowSize,

		AcceptingContracts: settings.AcceptingContracts,
		NumContracts:       srv.host.Contracts(),
		LostRevenue:        lostRevenue,
		Revenue:            revenue,
		StorageRemaining:   srv.host.Capacity(),
		AnticipatedRevenue: anticipatedRevenue,

		RPCErrorCalls:        rpcCalls.ErrorCalls,
		RPCUnrecognizedCalls: rpcCalls.UnrecognizedCalls,
		RPCDownloadCalls:     rpcCalls.DownloadCalls,
		RPCRenewCalls:        rpcCalls.RenewCalls,
		RPCReviseCalls:       rpcCalls.ReviseCalls,
		RPCSettingsCalls:     rpcCalls.SettingsCalls,
		RPCUploadCalls:       rpcCalls.UploadCalls,
	}
	writeJSON(w, hg)
}

// hostHandlerPOST handles POST request to the /host API endpoint.
func (srv *Server) hostHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Map each query string to a field in the host settings.
	settings := srv.host.Settings()
	qsVars := map[string]interface{}{
		"acceptingcontracts": &settings.AcceptingContracts,
		"collateral":         &settings.Collateral,
		"maxduration":        &settings.MaxDuration,
		"minduration":        &settings.MinDuration,
		"price":              &settings.Price,
		"totalstorage":       &settings.TotalStorage,
		"windowsize":         &settings.WindowSize,
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
	srv.host.SetSettings(settings)
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

// hostDeleteHandler deletes a file contract from the host.
func (srv *Server) hostDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	hash, err := scanAddress(ps.ByName("filecontractid"))
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	fcid := types.FileContractID(hash)
	err = srv.host.DeleteContract(fcid)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeSuccess(w)
}
