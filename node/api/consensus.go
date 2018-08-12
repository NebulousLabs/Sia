package api

import (
	"encoding/json"
	"fmt"
	"net/http"

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

// ConsensusHeadersGET contains information from a blocks header.
type ConsensusHeadersGET struct {
	BlockID types.BlockID `json:"blockid"`
}

// ConsensusBlocksGet contains all fields of a types.Block and additional
// fields for ID and Height.
type ConsensusBlocksGet struct {
	ID           types.BlockID           `json:"id"`
	Height       types.BlockHeight       `json:"height"`
	ParentID     types.BlockID           `json:"parentid"`
	Nonce        types.BlockNonce        `json:"nonce"`
	Timestamp    types.Timestamp         `json:"timestamp"`
	MinerPayouts []types.SiacoinOutput   `json:"minerpayouts"`
	Transactions []ConsensusBlocksGetTxn `json:"transactions"`
}

// ConsensusBlocksGetTxn contains all fields of a types.Transaction and an
// additional ID field.
type ConsensusBlocksGetTxn struct {
	ID                    types.TransactionID               `json:"id"`
	SiacoinInputs         []types.SiacoinInput              `json:"siacoininputs"`
	SiacoinOutputs        []ConsensusBlocksGetSiacoinOutput `json:"siacoinoutputs"`
	FileContracts         []ConsensusBlocksGetFileContract  `json:"filecontracts"`
	FileContractRevisions []types.FileContractRevision      `json:"filecontractrevisions"`
	StorageProofs         []types.StorageProof              `json:"storageproofs"`
	SiafundInputs         []types.SiafundInput              `json:"siafundinputs"`
	SiafundOutputs        []ConsensusBlocksGetSiafundOutput `json:"siafundoutputs"`
	MinerFees             []types.Currency                  `json:"minerfees"`
	ArbitraryData         [][]byte                          `json:"arbitrarydata"`
	TransactionSignatures []types.TransactionSignature      `json:"transactionsignatures"`
}

// ConsensusBlocksGetFileContract contains all fields of a types.FileContract
// and an additional ID field.
type ConsensusBlocksGetFileContract struct {
	ID                 types.FileContractID              `json:"id"`
	FileSize           uint64                            `json:"filesize"`
	FileMerkleRoot     crypto.Hash                       `json:"filemerkleroot"`
	WindowStart        types.BlockHeight                 `json:"windowstart"`
	WindowEnd          types.BlockHeight                 `json:"windowend"`
	Payout             types.Currency                    `json:"payout"`
	ValidProofOutputs  []ConsensusBlocksGetSiacoinOutput `json:"validproofoutputs"`
	MissedProofOutputs []ConsensusBlocksGetSiacoinOutput `json:"missedproofoutputs"`
	UnlockHash         types.UnlockHash                  `json:"unlockhash"`
	RevisionNumber     uint64                            `json:"revisionnumber"`
}

// ConsensusBlocksGetSiacoinOutput contains all fields of a types.SiacoinOutput
// and an additional ID field.
type ConsensusBlocksGetSiacoinOutput struct {
	ID         types.SiacoinOutputID `json:"id"`
	Value      types.Currency        `json:"value"`
	UnlockHash types.UnlockHash      `json:"unlockhash"`
}

// ConsensusBlocksGetSiafundOutput contains all fields of a types.SiafundOutput
// and an additional ID field.
type ConsensusBlocksGetSiafundOutput struct {
	ID         types.SiafundOutputID `json:"id"`
	Value      types.Currency        `json:"value"`
	UnlockHash types.UnlockHash      `json:"unlockhash"`
}

// ConsensusBlocksGetFromBlock is a helper method that uses a types.Block and
// types.BlockHeight to create a ConsensusBlocksGet object.
func consensusBlocksGetFromBlock(b types.Block, h types.BlockHeight) ConsensusBlocksGet {
	txns := make([]ConsensusBlocksGetTxn, 0, len(b.Transactions))
	for _, t := range b.Transactions {
		// Get the transaction's SiacoinOutputs.
		scos := make([]ConsensusBlocksGetSiacoinOutput, 0, len(t.SiacoinOutputs))
		for i, sco := range t.SiacoinOutputs {
			scos = append(scos, ConsensusBlocksGetSiacoinOutput{
				ID:         t.SiacoinOutputID(uint64(i)),
				Value:      sco.Value,
				UnlockHash: sco.UnlockHash,
			})
		}
		// Get the transaction's SiafundOutputs.
		sfos := make([]ConsensusBlocksGetSiafundOutput, 0, len(t.SiafundOutputs))
		for i, sfo := range t.SiafundOutputs {
			sfos = append(sfos, ConsensusBlocksGetSiafundOutput{
				ID:         t.SiafundOutputID(uint64(i)),
				Value:      sfo.Value,
				UnlockHash: sfo.UnlockHash,
			})
		}
		// Get the transaction's FileContracts.
		fcos := make([]ConsensusBlocksGetFileContract, 0, len(t.FileContracts))
		for i, fc := range t.FileContracts {
			// Get the FileContract's valid proof outputs.
			fcid := t.FileContractID(uint64(i))
			vpos := make([]ConsensusBlocksGetSiacoinOutput, 0, len(fc.ValidProofOutputs))
			for j, vpo := range fc.ValidProofOutputs {
				vpos = append(vpos, ConsensusBlocksGetSiacoinOutput{
					ID:         fcid.StorageProofOutputID(types.ProofValid, uint64(j)),
					Value:      vpo.Value,
					UnlockHash: vpo.UnlockHash,
				})
			}
			// Get the FileContract's missed proof outputs.
			mpos := make([]ConsensusBlocksGetSiacoinOutput, 0, len(fc.MissedProofOutputs))
			for j, mpo := range fc.MissedProofOutputs {
				mpos = append(mpos, ConsensusBlocksGetSiacoinOutput{
					ID:         fcid.StorageProofOutputID(types.ProofMissed, uint64(j)),
					Value:      mpo.Value,
					UnlockHash: mpo.UnlockHash,
				})
			}
			fcos = append(fcos, ConsensusBlocksGetFileContract{
				ID:                 fcid,
				FileSize:           fc.FileSize,
				FileMerkleRoot:     fc.FileMerkleRoot,
				WindowStart:        fc.WindowStart,
				WindowEnd:          fc.WindowEnd,
				Payout:             fc.Payout,
				ValidProofOutputs:  vpos,
				MissedProofOutputs: mpos,
				UnlockHash:         fc.UnlockHash,
				RevisionNumber:     fc.RevisionNumber,
			})
		}
		txns = append(txns, ConsensusBlocksGetTxn{
			ID:                    t.ID(),
			SiacoinInputs:         t.SiacoinInputs,
			SiacoinOutputs:        scos,
			FileContracts:         fcos,
			FileContractRevisions: t.FileContractRevisions,
			StorageProofs:         t.StorageProofs,
			SiafundInputs:         t.SiafundInputs,
			SiafundOutputs:        sfos,
			MinerFees:             t.MinerFees,
			ArbitraryData:         t.ArbitraryData,
			TransactionSignatures: t.TransactionSignatures,
		})
	}
	return ConsensusBlocksGet{
		ID:           b.ID(),
		Height:       h,
		ParentID:     b.ParentID,
		Nonce:        b.Nonce,
		Timestamp:    b.Timestamp,
		MinerPayouts: b.MinerPayouts,
		Transactions: txns,
	}
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
	var h types.BlockHeight
	var exists bool

	// Handle request by id
	if id != "" {
		var bid types.BlockID
		if err := bid.LoadString(id); err != nil {
			WriteError(w, Error{"failed to unmarshal blockid"}, http.StatusBadRequest)
			return
		}
		b, h, exists = api.cs.BlockByID(bid)
	}
	// Handle request by height
	if height != "" {
		if _, err := fmt.Sscan(height, &h); err != nil {
			WriteError(w, Error{"failed to parse block height"}, http.StatusBadRequest)
			return
		}
		b, exists = api.cs.BlockAtHeight(types.BlockHeight(h))
	}
	// Check if block was found
	if !exists {
		WriteError(w, Error{"block doesn't exist"}, http.StatusBadRequest)
		return
	}
	// Write response
	WriteJSON(w, consensusBlocksGetFromBlock(b, h))
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
