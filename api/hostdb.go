package api

import (
	"fmt"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

type (
	// HostdbActiveGET lists active hosts on the network.
	HostdbActiveGET struct {
		Hosts []modules.HostDBEntry `json:"hosts"`
	}

	// HostdbAllGET lists all hosts that the renter is aware of.
	HostdbAllGET struct {
		Hosts []modules.HostDBEntry `json:"hosts"`
	}

	// HostdbHostsGET lists detailed statistics for a particular host, selected
	// by pubkey.
	HostdbHostsGET struct {
		Entry          modules.HostDBEntry        `json:"entry"`
		ScoreBreakdown modules.HostScoreBreakdown `json:"scorebreakdown"`
	}
)

// hostdbActiveHandler handles the API call asking for the list of active
// hosts.
func (api *API) hostdbActiveHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
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

	WriteJSON(w, HostdbActiveGET{
		Hosts: hosts[:numHosts],
	})
}

// hostdbAllHandler handles the API call asking for the list of all hosts.
func (api *API) hostdbAllHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	WriteJSON(w, HostdbAllGET{
		Hosts: api.renter.AllHosts(),
	})
}

// hostdbHostsHandler handles the API call asking for a specific host,
// returning detailed informatino about that host.
func (api *API) hostdbHostsHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	var pk types.SiaPublicKey
	pk.LoadString(ps.ByName("pubkey"))

	entry, exists := api.renter.Host(pk)
	if !exists {
		WriteError(w, Error{"requested host does not exist"}, http.StatusBadRequest)
		return
	}
	breakdown := api.renter.ScoreBreakdown(entry)

	WriteJSON(w, HostdbHostsGET{
		Entry:          entry,
		ScoreBreakdown: breakdown,
	})
}
