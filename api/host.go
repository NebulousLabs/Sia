package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// HostGET contains the information that is returned after a GET request to
	// /host.
	HostGET struct {
		Collateral   types.Currency     `json:"collateral"`
		IPAddress    modules.NetAddress `json:"ipaddress"`
		MaxDuration  types.BlockHeight  `json:"maxduration"`
		MinDuration  types.BlockHeight  `json:"minduration"`
		Price        types.Currency     `json:"price"`
		TotalStorage int64              `json:"totalstorage"`
		UnlockHash   types.UnlockHash   `json:"unlockhash"`
		WindowSize   types.BlockHeight  `json:"windowsize"`

		NumContracts     uint64         `json:"numcontracts"`
		Revenue          types.Currency `json:"revenue"`
		StorageRemaining int64          `json:"storageremaining"`
		UpcomingRevenue  types.Currency `json:"upcomingrevenue"`
	}
)

// hostHandlerGET handles GET requests to the /host API endpoint.
func (srv *Server) hostHandlerGET(w http.ResponseWriter, req *http.Request) {
	settings := srv.host.Settings()
	upcomingRevenue, revenue := srv.host.Revenue()
	storageRemaining, numContracts := srv.host.Capacity()
	hg := HostGET{
		Collateral:   settings.Collateral,
		IPAddress:    settings.IPAddress,
		MaxDuration:  settings.MaxDuration,
		MinDuration:  settings.MinDuration,
		Price:        settings.Price,
		TotalStorage: settings.TotalStorage,
		UnlockHash:   settings.UnlockHash,
		WindowSize:   settings.WindowSize,

		NumContracts:     numContracts,
		Revenue:          revenue,
		StorageRemaining: storageRemaining,
		UpcomingRevenue:  upcomingRevenue,
	}
	writeJSON(w, hg)
}

// hostHandlerPOST handles POST request to the /host API endpoint.
func (srv *Server) hostHandlerPOST(w http.ResponseWriter, req *http.Request) {
	// Map each query string to a field in the host settings.
	settings := srv.host.Settings()
	qsVars := map[string]interface{}{
		"collateral":   &settings.Collateral,
		"maxduration":  &settings.MaxDuration,
		"minduration":  &settings.MinDuration,
		"price":        &settings.Price,
		"totalstorage": &settings.TotalStorage,
		"windowsize":   &settings.WindowSize,
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

// hostHandler handles the API call that queries the host status.
func (srv *Server) hostHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.hostHandlerGET(w, req)
	} else if req.Method == "POST" {
		srv.hostHandlerPOST(w, req)
	} else {
		writeError(w, "unrecognized method when calling /host", http.StatusBadRequest)
	}
}

// hostAnnounceHandlerPOST handles the API call that triggers a host
// announcement.
func (srv *Server) hostAnnounceHandlerPOST(w http.ResponseWriter, req *http.Request) {
	var err error
	if addr := req.FormValue("address"); addr != "" {
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

// hostAnnounceHandler handles the API call to get the host to announce itself
// to the network.
func (srv *Server) hostAnnounceHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		srv.hostAnnounceHandlerPOST(w, req)
	} else {
		writeError(w, "unrecognized method when calling /host/announce", http.StatusBadRequest)
	}
}
