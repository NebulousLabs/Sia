package api

import (
	"encoding/json"
	"net/http"
	"fmt"

	"github.com/NebulousLabs/Sia/crypto"
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

type ConsensusFileContract struct {
		FileSize           uint64                         `json:"filesize"`
		FileMerkleRoot     crypto.Hash                    `json:"filemerkleroot"`
		WindowStart        types.BlockHeight              `json:"windowstart"`
		WindowEnd          types.BlockHeight              `json:"windowend"`
		Payout             types.Currency                 `json:"payout"`
		ValidProofOutputs  map[string]types.SiacoinOutput `json:"validproofoutputs"`
		MissedProofOutputs map[string]types.SiacoinOutput `json:"missedproofoutputs"`
		UnlockHash         types.UnlockHash               `json:"unlockhash"`
		RevisionNumber     uint64                         `json:"revisionnumber"`
}

type ConsensusFileContractRevision struct {
		ParentID          types.FileContractID   `json:"parentid"`
		UnlockConditions  types.UnlockConditions `json:"unlockconditions"`
		NewRevisionNumber uint64                 `json:"newrevisionnumber"`

		NewFileSize           uint64                         `json:"newfilesize"`
		NewFileMerkleRoot     crypto.Hash                    `json:"newfilemerkleroot"`
		NewWindowStart        types.BlockHeight              `json:"newwindowstart"`
		NewWindowEnd          types.BlockHeight              `json:"newwindowend"`
		NewValidProofOutputs  map[string]types.SiacoinOutput `json:"newvalidproofoutputs"`
		NewMissedProofOutputs map[string]types.SiacoinOutput `json:"newmissedproofoutputs"`
		NewUnlockHash         types.UnlockHash               `json:"newunlockhash"`
}

type ConsensusTransaction struct {
		SiacoinInputs         map[string]types.SiacoinInput            `json:"siacoininputs"`
		SiacoinOutputs        map[string]types.SiacoinOutput           `json:"siacoinoutputs"`
		FileContracts         map[string]ConsensusFileContract         `json:"filecontracts"`
		FileContractRevisions map[string]ConsensusFileContractRevision `json:"filecontractrevisions"`
		StorageProofs         map[string]types.StorageProof            `json:"storageproofs"`
		SiafundInputs         map[string]types.SiafundInput            `json:"siafundinputs"`
		SiafundOutputs        map[string]types.SiafundOutput           `json:"siafundoutputs"`
		MinerFees             map[string]types.Currency                `json:"minerfees"`
		ArbitraryData         [][]byte                                 `json:"arbitrarydata"`
		TransactionSignatures map[string]types.TransactionSignature    `json:"transactionsignatures"`
}

// ConsensusBlockGET is the object returned by a GET request to
// /consensus/block.
type ConsensusBlock struct {
		BlockHeight types.BlockHeight `json:"blockheight"`
		BlockHeader types.BlockHeader `json:"blockheader"`

		MinerPayouts map[string]types.SiacoinOutput  `json:"minerpayouts"`
		Transactions map[string]ConsensusTransaction `json:"transactions"`
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

// consensusBlocksHandler handles API calls to /consensus/blocks/:height.
func (api *API) consensusBlocksHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse the height that's being requested.
	var height types.BlockHeight
	_, err := fmt.Sscan(ps.ByName("height"), &height)
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}

	// Fetch and return the explorer block.
	block, exists := api.cs.BlockAtHeight(height)
	if !exists {
		WriteError(w, Error{"no block found at input height in call to /consensus/blocks"}, http.StatusBadRequest)
		return
	}

	// Catalog the new miner payouts.
	minerpayouts := map[string]types.SiacoinOutput{}
	for j, payout := range block.MinerPayouts {
		scoid := block.MinerPayoutID(uint64(j)).String()
		minerpayouts[scoid] = payout
	}

	var ct = map[string]ConsensusTransaction{}

	// Update cumulative stats for applied transactions.
	for _, txn := range block.Transactions {
		// Add the transaction to the list of active transactions.
		txid := txn.ID()

		inputs := map[string]types.SiacoinInput{}
		for _, sci := range txn.SiacoinInputs {
			inputs[sci.ParentID.String()] = sci
		}

		outputs := map[string]types.SiacoinOutput{}
		for j, sco := range txn.SiacoinOutputs {
			scoid := txn.SiacoinOutputID(uint64(j)).String()
			outputs[scoid] = sco
		}

		filecontracts := map[string]ConsensusFileContract{}
		for k, fc := range txn.FileContracts {
			fcid := txn.FileContractID(uint64(k))

			validproofs := map[string]types.SiacoinOutput{}
			for l, sco := range fc.ValidProofOutputs {
				scoid := fcid.StorageProofOutputID(types.ProofValid, uint64(l)).String()
				validproofs[scoid] = sco
			}

			missedproofs := map[string]types.SiacoinOutput{}
			for l, sco := range fc.MissedProofOutputs {
				scoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(l)).String()
				missedproofs[scoid] = sco
			}

			filecontracts[fcid.String()] = ConsensusFileContract{
				FileSize:       fc.FileSize,
				FileMerkleRoot: fc.FileMerkleRoot,
				WindowStart:    fc.WindowStart,
				WindowEnd:      fc.WindowEnd,
				Payout:         fc.Payout,

				ValidProofOutputs:  validproofs,
				MissedProofOutputs: missedproofs,

				UnlockHash:     fc.UnlockHash,
				RevisionNumber: fc.RevisionNumber,
			}
		}

		filecontractrevisions := map[string]ConsensusFileContractRevision{}
		for _, fcr := range txn.FileContractRevisions {
			validproofs := map[string]types.SiacoinOutput{}
			for l, sco := range fcr.NewValidProofOutputs {
				scoid := fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l)).String()
				validproofs[scoid] = sco
			}

			missedproofs := map[string]types.SiacoinOutput{}
			for l, sco := range fcr.NewMissedProofOutputs {
				scoid := fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l)).String()
				missedproofs[scoid] = sco
			}

			filecontractrevisions[fcr.ParentID.String()] = ConsensusFileContractRevision{
				ParentID:          fcr.ParentID,
				UnlockConditions:  fcr.UnlockConditions,
				NewRevisionNumber: fcr.NewRevisionNumber,

				NewFileSize:       fcr.NewFileSize,
				NewFileMerkleRoot: fcr.NewFileMerkleRoot,
				NewWindowStart:    fcr.NewWindowStart,
				NewWindowEnd:      fcr.NewWindowEnd,

				NewValidProofOutputs:  validproofs,
				NewMissedProofOutputs: missedproofs,

				NewUnlockHash: fcr.NewUnlockHash,
			}
		}

		storageproofs := map[string]types.StorageProof{}
		for _, sp := range txn.StorageProofs {
			storageproofs[sp.ParentID.String()] = sp
		}

		sfinputs := map[string]types.SiafundInput{}
		for _, sfi := range txn.SiafundInputs {
			sfinputs[sfi.ParentID.String()] = sfi
		}

		sfoutputs := map[string]types.SiafundOutput{}
		for k, sfo := range txn.SiafundOutputs {
			sfoid := txn.SiafundOutputID(uint64(k)).String()
			sfoutputs[sfoid] = sfo
		}

		ct[txid.String()] = ConsensusTransaction{
            SiacoinInputs:  inputs,
    		SiacoinOutputs: outputs,
    		FileContracts: filecontracts,
    		FileContractRevisions: filecontractrevisions,
    		StorageProofs: storageproofs,
    		SiafundInputs: sfinputs,
    		SiafundOutputs: sfoutputs,
    		ArbitraryData: txn.ArbitraryData,
		}
	}
	WriteJSON(w, ConsensusBlock{
		BlockHeight:  height,
		BlockHeader:  block.Header(),
		Transactions: ct,
		MinerPayouts: minerpayouts,
	})
}
