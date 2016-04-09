package api

import (
	//"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

type (
	// HostGET contains the information that is returned after a GET request to
	// /host.
	HostGET struct {
		AcceptingContracts bool               `json:"acceptingcontracts"`
		MaxDuration        types.BlockHeight  `json:"maxduration"`
		NetAddress         modules.NetAddress `json:"netaddress"`
		RemainingStorage   uint64             `json:"remainingstorage"`
		TotalStorage       uint64             `json:"totalstorage"`
		UnlockHash         types.UnlockHash   `json:"unlockhash"`
		WindowSize         types.BlockHeight  `json:"windowsize"`

		Collateral             types.Currency `json:"collateral"`
		ContractPrice          types.Currency `json:"contractprice"`
		DownloadBandwidthPrice types.Currency `json:"downloadbandwidthprice"`
		StoragePrice           types.Currency `json:"storageprice"`
		UploadBandwidthPrice   types.Currency `json:"uploadbandwidthprice"`

		AnticipatedRevenue types.Currency `json:"anticipatedrevenue"`
		LostRevenue        types.Currency `json:"lostrevenue"`
		NumContracts       uint64         `json:"numcontracts"`
		Revenue            types.Currency `json:"revenue"`

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
	writeJSON(w, "TODO: fix/revamp this call")
	// settings := srv.host.Settings()
	// anticipatedRevenue, revenue, lostRevenue := srv.host.Revenue()
	// rpcCalls := srv.host.RPCMetrics()
	// hg := HostGET{
	// 	AcceptingContracts: settings.AcceptingContracts,
	// 	MaxDuration:        settings.MaxDuration,
	// 	NetAddress:         settings.NetAddress,
	// 	RemainingStorage:   srv.host.Capacity(),
	// 	TotalStorage:       settings.TotalStorage,
	// 	UnlockHash:         settings.UnlockHash,
	// 	WindowSize:         settings.WindowSize,

	// 	Collateral:             settings.Collateral,
	// 	ContractPrice:          settings.ContractPrice,
	// 	DownloadBandwidthPrice: settings.DownloadBandwidthPrice,
	// 	StoragePrice:           settings.StoragePrice,
	// 	UploadBandwidthPrice:   settings.UploadBandwidthPrice,

	// 	AnticipatedRevenue: anticipatedRevenue,
	// 	LostRevenue:        lostRevenue,
	// 	NumContracts:       srv.host.Contracts(),
	// 	Revenue:            revenue,

	// 	RPCErrorCalls:        rpcCalls.ErrorCalls,
	// 	RPCUnrecognizedCalls: rpcCalls.UnrecognizedCalls,
	// 	RPCDownloadCalls:     rpcCalls.DownloadCalls,
	// 	RPCRenewCalls:        rpcCalls.RenewCalls,
	// 	RPCReviseCalls:       rpcCalls.ReviseCalls,
	// 	RPCSettingsCalls:     rpcCalls.SettingsCalls,
	// 	RPCUploadCalls:       rpcCalls.UploadCalls,
	// }
	// writeJSON(w, hg)
}

// hostHandlerPOST handles POST request to the /host API endpoint.
func (srv *Server) hostHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, "TODO: fix/revamp this call")
	// // Map each query string to a field in the host settings.
	// settings := srv.host.Settings()
	// qsVars := map[string]interface{}{
	// 	// TODO: I'm not sure that allowing the user to set the netaddress via
	// 	// SetSettings is a good idea, if they change it to a bad address, or
	// 	// even a dns address... what happens?
	// 	"acceptingcontracts": &settings.AcceptingContracts,
	// 	"maxduration":        &settings.MaxDuration,
	// 	"netaddress":         &settings.NetAddress,
	// 	"windowsize":         &settings.WindowSize,

	// 	"collateral":             &settings.Collateral,
	// 	"contractprice":          &settings.ContractPrice,
	// 	"downloadbandwidthprice": &settings.DownloadBandwidthPrice,
	// 	"StoragePrice":           &settings.StoragePrice,
	// 	"uploadbandwidthprice":   &settings.UploadBandwidthPrice,
	// }

	// // Iterate through the query string and replace any fields that have been
	// // altered.
	// for qs := range qsVars {
	// 	if req.FormValue(qs) != "" { // skip empty values
	// 		_, err := fmt.Sscan(req.FormValue(qs), qsVars[qs])
	// 		if err != nil {
	// 			writeError(w, "Malformed "+qs, http.StatusBadRequest)
	// 			return
	// 		}
	// 	}
	// }
	// srv.host.SetSettings(settings)
	// writeSuccess(w)
}

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (srv *Server) hostAnnounceHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeJSON(w, "TODO: fix/revamp this call")
	// var err error
	// // TODO: should it just announce using the address found in host.Settings?
	// // What happens if the two are different?
	// if addr := req.FormValue("netaddress"); addr != "" {
	// 	err = srv.host.AnnounceAddress(modules.NetAddress(addr))
	// } else {
	// 	err = srv.host.Announce()
	// }
	// if err != nil {
	// 	writeError(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	// writeSuccess(w)
}

// hostDeleteHandler deletes a file contract from the host.
func (srv *Server) hostDeleteHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	writeJSON(w, "TODO: fix/revamp this call")
	// hash, err := scanAddress(ps.ByName("filecontractid"))
	// if err != nil {
	// 	writeError(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	// fcid := types.FileContractID(hash)
	// err = srv.host.DeleteContract(fcid)
	// if err != nil {
	// 	writeError(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }
	// writeSuccess(w)
}
