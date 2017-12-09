package api

import (
	"fmt"
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
	"strconv"
)

const (
	//MaxBlocksRequest is the maximum number of blocks a client can request.  10 is chosen
	MaxBlocksRequest = 10
)

//explorerTxSubscribe Handles the upgrade from HTTP -> WS, creates a new subscriber and starts the socket writer
func (api *API) explorerBlockSubscribe(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	conn, err := Upgrader.Upgrade(w, req, nil)
	if err != nil && build.DEBUG {
		log.Printf("Unable to upgrade request. Error: %s", err)
		return
	}
	subscriber := &Subscriber{hub: api.hub, conn: conn, send: make(chan []byte, 256)}
	subscriber.hub.registerBlock <- subscriber
	go subscriber.SocketWriter()
}

//explorerTxSubscribe Handles the upgrade from HTTP -> WS, creates a new subscriber and starts the socket writer
func (api *API) explorerTxSubscribe(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	conn, err := Upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Println(err)
		return
	}

	subscriber := &Subscriber{hub: api.hub, conn: conn, send: make(chan []byte, 256)}
	subscriber.hub.registerTx <- subscriber
	go subscriber.SocketWriter()
}

type (
	// ExplorerBlock is a block with some extra information such as the id and
	// height. This information is provided for programs that may not be
	// complex enough to compute the ID on their own.
	ExplorerBlock struct {
		MinerPayoutIDs []types.SiacoinOutputID `json:"minerpayoutids,omitempty"`
		Transactions   []ExplorerTransaction   `json:"transactions"`
		RawBlock       types.Block             `json:"rawblock,omitempty"`
		modules.BlockFacts
	}

	//PendingExplorerBlock is a block with the current facts and list of transactions.  It doesn't
	//have an ID yet since it's still in progress
	PendingExplorerBlock struct {
		Transactions []ExplorerTransaction `json:"transactions"`
		modules.BlockFacts
	}

	// ExplorerTransaction is a transcation with some extra information such as
	// the parent block. This information is provided for programs that may not
	// be complex enough to compute the extra information on their own.
	ExplorerTransaction struct {
		ID                                       types.TransactionID       `json:"id"`
		Height                                   types.BlockHeight         `json:"height"`
		Parent                                   types.BlockID             `json:"parent"`
		RawTransaction                           types.Transaction         `json:"rawtransaction"`
		SiacoinInputOutputs                      []types.SiacoinOutput     `json:"siacoininputoutputs"` // the outputs being spent
		SiacoinOutputIDs                         []types.SiacoinOutputID   `json:"siacoinoutputids"`
		FileContractIDs                          []types.FileContractID    `json:"filecontractids"`
		FileContractValidProofOutputIDs          [][]types.SiacoinOutputID `json:"filecontractvalidproofoutputids"`          // outer array is per-contract
		FileContractMissedProofOutputIDs         [][]types.SiacoinOutputID `json:"filecontractmissedproofoutputids"`         // outer array is per-contract
		FileContractRevisionValidProofOutputIDs  [][]types.SiacoinOutputID `json:"filecontractrevisionvalidproofoutputids"`  // outer array is per-revision
		FileContractRevisionMissedProofOutputIDs [][]types.SiacoinOutputID `json:"filecontractrevisionmissedproofoutputids"` // outer array is per-revision
		StorageProofOutputIDs                    [][]types.SiacoinOutputID `json:"storageproofoutputids"`                    // outer array is per-payout
		StorageProofOutputs                      [][]types.SiacoinOutput   `json:"storageproofoutputs"`                      // outer array is per-payout
		SiafundInputOutputs                      []types.SiafundOutput     `json:"siafundinputoutputs"`                      // the outputs being spent
		SiafundOutputIDs                         []types.SiafundOutputID   `json:"siafundoutputids"`
		SiafundClaimOutputIDs                    []types.SiacoinOutputID   `json:"siafundclaimoutputids"`
	}

	// ExplorerBlockGET is the object returned by a GET request to
	// /explorer/pending.
	ExplorerBlockGET struct {
		Blocks []ExplorerBlock `json:"blocks"`
	}

	// ExplorerPendingBlockGET is the object returned by a GET request to
	// /explorer/pending.
	ExplorerPendingBlockGET struct {
		PendingBlock PendingExplorerBlock `json:"block"`
	}

	// ExplorerConsensusChange is pushed to the websocket when a consensus change event occurs
	// This allows subscribers to listen and react without having to poll the API
	ExplorerConsensusChange struct {
		AppliedBlocks  []ExplorerBlock `json:"applied_blocks"`
		RevertedBlocks []types.BlockID `json:"reverted_blocks"`
	}

	// ExplorerUnconfirmedTransactionChange is pushed to the websocket when a transaction set update occurs
	// This allows subscribers to listen and react without having to poll the API
	ExplorerUnconfirmedTransactionChange struct {
		AppliedTransactions  []ExplorerTransaction `json:"applied_txs"`
		RevertedTransactions []types.TransactionID `json:"reverted_txs"`
	}

	// ExplorerHashGET is the object returned as a response to a GET request to
	// /explorer/hash. The HashType will indicate whether the hash corresponds
	// to a block id, a transaction id, a siacoin output id, a file contract
	// id, or a siafund output id. In the case of a block id, 'Block' will be
	// filled out and all the rest of the fields will be blank. In the case of
	// a transaction id, 'Transaction' will be filled out and all the rest of
	// the fields will be blank. For everything else, 'Transactions' and
	// 'Blocks' will/may be filled out and everything else will be blank.
	ExplorerHashGET struct {
		HashType     string                `json:"hashtype"`
		Block        *ExplorerBlock        `json:"block"`
		Blocks       []ExplorerBlock       `json:"blocks"`
		Transaction  *ExplorerTransaction  `json:"transaction"`
		Transactions []ExplorerTransaction `json:"transactions"`
	}
)

// buildExplorerTransaction takes a transaction and the height + id of the
// block it appears in an uses that to build an explorer transaction.
func (api *API) buildExplorerTransaction(height types.BlockHeight, parent types.BlockID, txn types.Transaction) (et ExplorerTransaction) {
	// Get the header information for the transaction.
	et.ID = txn.ID()
	et.Height = height
	et.Parent = parent
	et.RawTransaction = txn

	// Add the siacoin outputs that correspond with each siacoin input.
	for _, sci := range txn.SiacoinInputs {
		sco, exists := api.explorer.SiacoinOutput(sci.ParentID)
		if build.DEBUG && !exists {
			log.Println("could not find corresponding siacoin output")
		}
		et.SiacoinInputOutputs = append(et.SiacoinInputOutputs, sco)
	}

	for i := range txn.SiacoinOutputs {
		et.SiacoinOutputIDs = append(et.SiacoinOutputIDs, txn.SiacoinOutputID(uint64(i)))
	}

	// Add all of the valid and missed proof ids as extra data to the file
	// contracts.
	for i, fc := range txn.FileContracts {
		fcid := txn.FileContractID(uint64(i))
		var fcvpoids []types.SiacoinOutputID
		var fcmpoids []types.SiacoinOutputID
		for j := range fc.ValidProofOutputs {
			fcvpoids = append(fcvpoids, fcid.StorageProofOutputID(types.ProofValid, uint64(j)))
		}
		for j := range fc.MissedProofOutputs {
			fcmpoids = append(fcmpoids, fcid.StorageProofOutputID(types.ProofMissed, uint64(j)))
		}
		et.FileContractIDs = append(et.FileContractIDs, fcid)
		et.FileContractValidProofOutputIDs = append(et.FileContractValidProofOutputIDs, fcvpoids)
		et.FileContractMissedProofOutputIDs = append(et.FileContractMissedProofOutputIDs, fcmpoids)
	}

	// Add all of the valid and missed proof ids as extra data to the file
	// contract revisions.
	for _, fcr := range txn.FileContractRevisions {
		var fcrvpoids []types.SiacoinOutputID
		var fcrmpoids []types.SiacoinOutputID
		for j := range fcr.NewValidProofOutputs {
			fcrvpoids = append(fcrvpoids, fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(j)))
		}
		for j := range fcr.NewMissedProofOutputs {
			fcrmpoids = append(fcrmpoids, fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(j)))
		}
		et.FileContractValidProofOutputIDs = append(et.FileContractValidProofOutputIDs, fcrvpoids)
		et.FileContractMissedProofOutputIDs = append(et.FileContractMissedProofOutputIDs, fcrmpoids)
	}

	// Add all of the output ids and outputs corresponding with each storage
	// proof.
	for _, sp := range txn.StorageProofs {
		fileContract, fileContractRevisions, fileContractExists, _ := api.explorer.FileContractHistory(sp.ParentID)
		if !fileContractExists && build.DEBUG {
			log.Println("could not find a file contract connected with a storage proof")
		}
		var storageProofOutputs []types.SiacoinOutput
		if len(fileContractRevisions) > 0 {
			storageProofOutputs = fileContractRevisions[len(fileContractRevisions)-1].NewValidProofOutputs
		} else {
			storageProofOutputs = fileContract.ValidProofOutputs
		}
		var storageProofOutputIDs []types.SiacoinOutputID
		for i := range storageProofOutputs {
			storageProofOutputIDs = append(storageProofOutputIDs, sp.ParentID.StorageProofOutputID(types.ProofValid, uint64(i)))
		}
		et.StorageProofOutputIDs = append(et.StorageProofOutputIDs, storageProofOutputIDs)
		et.StorageProofOutputs = append(et.StorageProofOutputs, storageProofOutputs)
	}

	// Add the siafund outputs that correspond to each siacoin input.
	for _, sci := range txn.SiafundInputs {
		sco, exists := api.explorer.SiafundOutput(sci.ParentID)
		if build.DEBUG && !exists {
			log.Println("could not find corresponding siafund output")
		}
		et.SiafundInputOutputs = append(et.SiafundInputOutputs, sco)
	}

	for i := range txn.SiafundOutputs {
		et.SiafundOutputIDs = append(et.SiafundOutputIDs, txn.SiafundOutputID(uint64(i)))
	}

	for _, sfi := range txn.SiafundInputs {
		et.SiafundClaimOutputIDs = append(et.SiafundClaimOutputIDs, sfi.ParentID.SiaClaimOutputID())
	}
	return et
}

// buildExplorerBlock takes a block and its height and uses it to construct an
// explorer block.
func (api *API) buildExplorerBlock(height types.BlockHeight, block types.Block) ExplorerBlock {
	var mpoids []types.SiacoinOutputID
	for i := range block.MinerPayouts {
		mpoids = append(mpoids, block.MinerPayoutID(uint64(i)))
	}

	var etxns []ExplorerTransaction
	for _, txn := range block.Transactions {
		etxns = append(etxns, api.buildExplorerTransaction(height, block.ID(), txn))
	}

	facts, exists := api.explorer.BlockFacts(height)
	if build.DEBUG && !exists {
		panic("incorrect request to buildExplorerBlock - block does not exist")
	} else if !exists {
		log.Printf("incorrect request to buildExplorerBlock - block does not exist")
		return ExplorerBlock{}
	}

	return ExplorerBlock{
		MinerPayoutIDs: mpoids,
		Transactions:   etxns,
		RawBlock:       block,

		BlockFacts: facts,
	}
}

// explorerHandler handles API calls to /explorer/blocks/:height.
func (api *API) explorerBlocksHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Parse the height that's being requested.
	var height types.BlockHeight
	_, err := fmt.Sscan(ps.ByName("height"), &height)
	//if the height is not found in the params, looks for to/from.
	if err != nil {
		queryValues := req.URL.Query()
		fromStr := queryValues.Get("from")
		//if the "height" is not found in the params and "from" is not found in the params, throw an error
		if fromStr == "" {
			WriteError(w, Error{fmt.Sprintf("Must provide a 'from' parameter when not specifiying 'height' ")}, http.StatusBadRequest)
			return
		}
		fromInt, err := strconv.Atoi(fromStr)
		if err != nil {
			WriteError(w, Error{fmt.Sprintf("Invalid 'from' parameter: %s.  Error: %s", fromStr, err)}, http.StatusBadRequest)
			return
		}
		from := types.BlockHeight(fromInt)

		toStr := queryValues.Get("to")
		var to types.BlockHeight
		//if "to" isn't found, set "to" equal to the Height
		if toStr == "" {
			to = api.cs.Height()
		} else {
			toInt, err := strconv.Atoi(toStr)
			if err != nil {
				WriteError(w, Error{fmt.Sprintf("Invalid 'to' parameter: %s.  Error: %s", fromStr, err)}, http.StatusBadRequest)
				return
			}
			to = types.BlockHeight(toInt)
		}

		//if from > to, throw an exception, because that's impossible query condition
		if from > to {
			WriteError(w, Error{"from paramter must be less than the to parameter"}, http.StatusBadRequest)
			return
		}

		//if "to" is greater than the current consensus Height, that's an impossible query condition
		if to > api.cs.Height() {
			WriteError(w, Error{fmt.Sprintf("to paramater must be less than the current block height of: %s", api.cs.Height())}, http.StatusBadRequest)
			return
		}

		if to-from > MaxBlocksRequest {
			WriteError(w, Error{fmt.Sprintf("to paramater must be less than the current block height of: %s", api.cs.Height())}, http.StatusBadRequest)
			return
		}

		var blockGet ExplorerBlockGET
		for blockHeight := from; blockHeight <= to; blockHeight++ {
			block, exists := api.cs.BlockAtHeight(blockHeight)
			if !exists {
				WriteError(w, Error{fmt.Sprintf("no block found at height %s in call to /explorer/block.  This is a server error, please contact the operator of this explorer", blockHeight)}, http.StatusInternalServerError)
				return
			}
			blockGet.Blocks = append(blockGet.Blocks, api.buildExplorerBlock(blockHeight, block))
		}
		// Fetch and return the explorer blocks.
		WriteJSON(w, blockGet)
		return
	} else {
		// Fetch and return the explorer block.
		block, exists := api.cs.BlockAtHeight(height)
		if !exists {
			WriteError(w, Error{"no block found at input height in call to /explorer/block"}, http.StatusBadRequest)
			return
		}
		WriteJSON(w, ExplorerBlockGET{
			Blocks: []ExplorerBlock{api.buildExplorerBlock(height, block)},
		})
		return
	}
}

// buildTransactionSet returns the blocks and transactions that are associated
// with a set of transaction ids.
func (api *API) buildTransactionSet(txids []types.TransactionID) (txns []ExplorerTransaction, blocks []ExplorerBlock) {
	for _, txid := range txids {
		// Get the block containing the transaction - in the case of miner
		// payouts, the block might be the transaction.
		block, height, exists := api.explorer.Transaction(txid)
		if !exists && build.DEBUG {
			panic("explorer pointing to nonexistent txn")
		}

		// Check if the block is the transaction.
		if types.TransactionID(block.ID()) == txid {
			blocks = append(blocks, api.buildExplorerBlock(height, block))
		} else {
			// Find the transaction within the block with the correct id.
			for _, t := range block.Transactions {
				if t.ID() == txid {
					txns = append(txns, api.buildExplorerTransaction(height, block.ID(), t))
					break
				}
			}
		}
	}
	return txns, blocks
}

// explorerHashHandler handles GET requests to /explorer/hash/:hash.
func (api *API) explorerHashHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	// Scan the hash as a hash. If that fails, try scanning the hash as an
	// address.
	hash, err := scanHash(ps.ByName("hash"))
	if err != nil {
		addr, err := scanAddress(ps.ByName("hash"))
		if err != nil {
			WriteError(w, Error{err.Error()}, http.StatusBadRequest)
			return
		}
		hash = crypto.Hash(addr)
	}

	// TODO: lookups on the zero hash are too expensive to allow. Need a
	// better way to handle this case.
	if hash == (crypto.Hash{}) {
		WriteError(w, Error{"can't lookup the empty unlock hash"}, http.StatusBadRequest)
		return
	}

	hashType, err := api.explorer.HashType(hash)

	if err != nil {
		WriteError(w, Error{fmt.Sprintf("hash not found in hashtype db.  %s", err)}, http.StatusBadRequest)
		return
	}

	switch hashType {
	case modules.BlockHashType:
		{
			block, height, exists := api.explorer.Block(types.BlockID(hash))
			if exists {
				b := api.buildExplorerBlock(height, block)
				WriteJSON(w, ExplorerHashGET{
					HashType: "blockid",
					Block:    &b,
				})
				return
			} else {
				WriteError(w, Error{"hash found to be a Block HashType, but not found in database"}, http.StatusInternalServerError)
			}
		}
	case modules.TransactionHashType:
		{
			block, height, exists := api.explorer.Transaction(types.TransactionID(hash))
			if exists {
				var txn types.Transaction
				for _, t := range block.Transactions {
					if t.ID() == types.TransactionID(hash) {
						txn = t
					}
				}
				tx := api.buildExplorerTransaction(height, block.ID(), txn)
				WriteJSON(w, ExplorerHashGET{
					HashType:    "transactionid",
					Transaction: &tx,
				})
				return
			} else {
				WriteError(w, Error{"hash found to be a Transaction HashType, but not found in database"}, http.StatusInternalServerError)
			}
		}
	case modules.SiacoinOutputIdHashType:
		{

			txids := api.explorer.SiacoinOutputID(types.SiacoinOutputID(hash))
			if len(txids) != 0 {
				txns, blocks := api.buildTransactionSet(txids)
				WriteJSON(w, ExplorerHashGET{
					HashType:     "siacoinoutputid",
					Blocks:       blocks,
					Transactions: txns,
				})
				return
			} else {
				WriteError(w, Error{"hash found to be a SiacoinOutputId HashType, but not found in database"}, http.StatusInternalServerError)
			}
		}
	case modules.FileContractIdHashType:
		{
			txids := api.explorer.FileContractID(types.FileContractID(hash))
			if len(txids) != 0 {
				txns, blocks := api.buildTransactionSet(txids)
				WriteJSON(w, ExplorerHashGET{
					HashType:     "filecontractid",
					Blocks:       blocks,
					Transactions: txns,
				})
				return
			} else {
				WriteError(w, Error{"hash found to be a FileContractId HashType, but not found in database"}, http.StatusInternalServerError)

			}
		}
	case modules.SiafundOutputIdHashType:
		{
			txids := api.explorer.SiafundOutputID(types.SiafundOutputID(hash))
			if len(txids) != 0 {
				txns, blocks := api.buildTransactionSet(txids)
				WriteJSON(w, ExplorerHashGET{
					HashType:     "siafundoutputid",
					Blocks:       blocks,
					Transactions: txns,
				})
				return
			} else {
				WriteError(w, Error{"hash found to be a SiafundOutputId HashType, but not found in database"}, http.StatusInternalServerError)
			}
		}
	}

	// Try the hash as an unlock hash. Unlock hash is checked last because
	// unlock hashes do not have collision-free guarantees. Someone can create
	// an unlock hash that collides with another object id. They will not be
	// able to use the unlock hash, but they can disrupt the explorer. This is
	// handled by checking the unlock hash last. Anyone intentionally creating
	// a colliding unlock hash (such a collision can only happen if done
	// intentionally) will be unable to find their unlock hash in the
	// blockchain through the explorer hash lookup.
	txids := api.explorer.UnlockHash(types.UnlockHash(hash))
	if len(txids) != 0 {
		txns, blocks := api.buildTransactionSet(txids)
		WriteJSON(w, ExplorerHashGET{
			HashType:     "unlockhash",
			Blocks:       blocks,
			Transactions: txns,
		})
		return
	}

	// Hash not found, return an error.
	WriteError(w, Error{"unrecognized hash used as input to /explorer/hash"}, http.StatusBadRequest)
}

// explorerHandler handles API calls to /explorer which returns the current block
func (api *API) explorerHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	facts := api.explorer.LatestBlockFacts()
	// Fetch and return the explorer block.
	block, exists := api.cs.BlockAtHeight(facts.Height)
	if !exists {
		WriteError(w, Error{"no block found at input height in call to /explorer/block"}, http.StatusBadRequest)
		return
	}

	WriteJSON(w, ExplorerBlockGET{
		Blocks: []ExplorerBlock{api.buildExplorerBlock(facts.Height, block)},
	})
}

// pendingBlockHandler handles API calls to /explorer/pending which returns the pending block
func (api *API) pendingBlockHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	cb := api.cs.CurrentBlock()

	pendingTxs := api.explorer.PendingTransactions()
	var explorerReps []ExplorerTransaction
	for _, ptx := range pendingTxs {
		explorerReps = append(explorerReps, api.buildExplorerTransaction(api.cs.Height()+1, cb.ParentID, ptx))
	}

	facts := api.explorer.LatestBlockFacts()
	WriteJSON(w, ExplorerBlockGET{
		Blocks: []ExplorerBlock{ExplorerBlock{
			BlockFacts:   facts,
			Transactions: explorerReps,
		}},
	})
}
