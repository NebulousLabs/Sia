package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/NebulousLabs/Sia/types"

	"github.com/julienschmidt/httprouter"
)

// ConsensusGET contains general information about the consensus set, with tags
// to support idiomatic json encodings.
type ConsensusGET struct {
	Synced       bool              `json:"synced"`
	Height       types.BlockHeight `json:"height"`
	CurrentBlock types.BlockID     `json:"currentblock"`
	Target       types.Target      `json:"target"`
	Difficulty   types.Currency    `json:"difficulty"`
}

// ConsensusHeadersGET contains information from a blocks header.
type ConsensusHeadersGET struct {
	BlockID types.BlockID `json:"blockid"`
}

// consensusHandler handles the API calls to /consensus.
func (api *API) consensusHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	cbid := api.cs.CurrentBlock().ID()
	currentTarget, _ := api.cs.ChildTarget(cbid)
	WriteJSON(w, ConsensusGET{
		Synced:       api.cs.Synced(),
		Height:       api.cs.Height(),
		CurrentBlock: cbid,
		Target:       currentTarget,
		Difficulty:   currentTarget.Difficulty(),
	})
}

// consensusBlocksIDHandler handles the API calls to
// /consensus/blocks/:id endpoint.
func (api *API) consensusBlocksIDHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Unmarshal BlockID
	var id types.BlockID
	if err := id.LoadString(ps.ByName("id")); err != nil {
		println(err.Error())
		WriteError(w, Error{"failed to unmarshal blockid"}, http.StatusBadRequest)
		return
	}
	// Retrieve block from consensus
	b, exists := api.cs.BlockByID(id)
	if !exists {
		WriteError(w, Error{fmt.Sprintf("block with id %v doesn't exist", id)}, http.StatusBadRequest)
		return
	}
	// Write response
	WriteJSON(w, b)
}

// consensusHeadersHeightHandler handles the API calls to
// consensus/headers/:height endpoint.
func (api *API) consensusHeadersHeightHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse height
	height, err := strconv.ParseUint(ps.ByName("height"), 10, 64)
	if err != nil {
		WriteError(w, Error{"failed to parse height"}, http.StatusBadRequest)
		return
	}
	// Retrieve block at height
	b, exists := api.cs.BlockAtHeight(types.BlockHeight(height))
	if !exists {
		WriteError(w, Error{fmt.Sprintf("could not find block at height %v", height)}, http.StatusBadRequest)
		return
	}
	// Write reponse
	WriteJSON(w, ConsensusHeadersGET{
		BlockID: b.ID(),
	})
}

// consensusValidateTransactionsetHandler handles the API calls to
// /consensus/validate/transactionset.
func (api *API) consensusValidateTransactionsetHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var txnset []types.Transaction
	err := json.NewDecoder(req.Body).Decode(&txnset)
	if err != nil {
		WriteError(w, Error{"could not decode transaction set: " + err.Error()}, http.StatusBadRequest)
		return
	}
	_, err = api.cs.TryTransactionSet(txnset)
	if err != nil {
		WriteError(w, Error{"transaction set validation failed: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}
