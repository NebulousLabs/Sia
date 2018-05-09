package api

import (
	"encoding/json"
	"fmt"
	"net/http"

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

type ConsensusBlocksGet struct {
	ID               types.BlockID                              `json:"id"`
	Height           types.BlockHeight                          `json:"height"`
	ParentID         types.BlockID                              `json:"parentid"`
	Nonce            types.BlockNonce                           `json:"nonce"`
	Timestamp        types.Timestamp                            `json:"timestamp"`
	MinerPayouts     []types.SiacoinOutput                      `json:"minerpayouts"`
	Transactions     []types.Transaction                        `json:"transactions"`
	TransactionIDs   []types.TransactionID                      `json:"transactionids"`
	SiacoinOutputIDs map[string]types.SiacoinOutputID `json:"siacoinoutputids"`
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

// consensusBlocksIDHandler handles the API calls to /consensus/blocks
// endpoint.
func (api *API) consensusBlocksHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Get query params and check them.
	id, height := req.FormValue("id"), req.FormValue("height")
	if id != "" && height != "" {
		WriteError(w, Error{"can't specify both id and height"}, http.StatusBadRequest)
	}
	if id == "" && height == "" {
		WriteError(w, Error{"either id or height has to be provided"}, http.StatusBadRequest)
	}

	var b types.Block
	var exists bool
	var blockheight types.BlockHeight

	// Handle request by id
	if id != "" {
		var bid types.BlockID
		if err := bid.LoadString(id); err != nil {
			WriteError(w, Error{"failed to unmarshal blockid"}, http.StatusBadRequest)
			return
		}
		b, blockheight, exists = api.cs.BlockByID(bid)
	}
	// Handle request by height
	if height != "" {
		var h uint64
		if _, err := fmt.Sscan(height, &h); err != nil {
			WriteError(w, Error{"failed to parse block height"}, http.StatusBadRequest)
			return
		}
		b, exists = api.cs.BlockAtHeight(types.BlockHeight(h))
		blockheight = types.BlockHeight(h)
	}
	// Check if block was found
	if !exists {
		WriteError(w, Error{"block doesn't exist"}, http.StatusBadRequest)
		return
	}

	var transactionIDs []types.TransactionID
	siacoinOutputIDs := make(map[string]types.SiacoinOutputID)

	for _, txn := range b.Transactions {
		transactionIDs = append(transactionIDs, txn.ID())
		for j := range txn.SiacoinOutputs {
			siacoinOutputIDs[txn.SiacoinOutputs[j].UnlockHash.String()] = txn.SiacoinOutputID(uint64(j))
		}
	}



	//for i,txn := range b.MinerPayouts {
	//	unlockHash = b.MinerPayouts[i].UnlockHash
	//	outputID = b.MinerPayouts[i] ?
	//}

	// Write response
	WriteJSON(w, ConsensusBlocksGet{
		b.ID(),
		blockheight,
		b.ParentID,
		b.Nonce,
		b.Timestamp,
		b.MinerPayouts,
		b.Transactions,
		transactionIDs,
		siacoinOutputIDs,
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
