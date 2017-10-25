package api

import (
	"encoding/json"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
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

// ConsensusChangeGet contains a raw consensus set with tags to support idiomatic json encodings.
type ConsensusChangeGet struct {
	ID                         modules.ConsensusChangeID          `json:"id"`
	NextID                     modules.ConsensusChangeID          `json:"nextid"`
	RevertedBlocks             []types.Block                      `json:"revertedblocks"`
	AppliedBlocks              []types.Block                      `json:"appliedblocks"`
	SiacoinOutputDiffs         []modules.SiacoinOutputDiff        `json:"siacoinoutputdiffs"`
	FileContractDiffs          []modules.FileContractDiff         `json:"filecontractdiffs"`
	SiafundOutputDiffs         []modules.SiafundOutputDiff        `json:"siafundoutputdiffs"`
	DelayedSiacoinOutputDiffs  []modules.DelayedSiacoinOutputDiff `json:"delayedsiacoinoutputdiffs"`
	SiafundPoolDiffs           []modules.SiafundPoolDiff          `json:"siafundpooldiffs"`
	ChildTarget                types.Target                       `json:"childtarget"`
	MinimumValidChildTimestamp types.Timestamp                    `json:"minimumvalidchildtimestamp"`
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

// consensusChange handles the API calls to /consensus/change
func (api *API) consensusChange(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse the changeid that's being requested.
	var id modules.ConsensusChangeID
	err := id.LoadString(ps.ByName("id"))
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	cc, next, err := api.cs.ConsensusChange(id)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	// Return the consensus change
	WriteJSON(w, ConsensusChangeGet{
		ID:                         cc.ID,
		NextID:                     next,
		RevertedBlocks:             cc.RevertedBlocks,
		AppliedBlocks:              cc.AppliedBlocks,
		SiacoinOutputDiffs:         cc.SiacoinOutputDiffs,
		FileContractDiffs:          cc.FileContractDiffs,
		SiafundOutputDiffs:         cc.SiafundOutputDiffs,
		DelayedSiacoinOutputDiffs:  cc.DelayedSiacoinOutputDiffs,
		SiafundPoolDiffs:           cc.SiafundPoolDiffs,
		ChildTarget:                cc.ChildTarget,
		MinimumValidChildTimestamp: cc.MinimumValidChildTimestamp,
	})
}
