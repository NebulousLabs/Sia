package api

import (
	"net/http"

	"github.com/NebulousLabs/Sia/types"
)

type (
	// ExplorerGET is the object returned as a response to a GET request to
	// /explorer.
	ExplorerGET struct {
		// General consensus information.
		Height            types.BlockHeight `json:"height"`
		Block             types.BlockID     `json:"block"`
		Target            types.Target      `json:"target"`
		Difficulty        types.Currency    `json:"difficulty"`
		MaturityTimestamp types.Timestamp   `json:"maturitytimestamp"`
		Circulation       types.Currency    `json:"circulation"`

		// Information about transaction type usage.
		TransactionCount          uint64 `json:"transactioncount"`
		SiacoinInputCount         uint64 `json:"siacoininputcount"`
		SiacoinOutputCount        uint64 `json:"siacoinoutputcount"`
		FileContractCount         uint64 `json:"filecontractcount"`
		FileContractRevisionCount uint64 `json:"filecontractrevisioncount"`
		StorageProofCount         uint64 `json:"storageproofcount"`
		SiafundInputCount         uint64 `json:"siafundinputcount"`
		SiafundOutputCount        uint64 `json:"siafundoutputcount"`
		MinerFeeCount             uint64 `json:"minerfeecount"`
		ArbitraryDataCount        uint64 `json:"arbitrarydatacount"`
		TransactionSignatureCount uint64 `json:"transactionsignaturecount"`

		// Information about file contracts and file contract revisions.
		ActiveContractCount uint64         `json:"activecontractcount"`
		ActiveContractCost  types.Currency `json:"activecontractcost"`
		ActiveContractSize  types.Currency `json:"activecontractsize"`
		TotalContractCost   types.Currency `json:"totalcontractcost"`
		TotalContractSize   types.Currency `json:"totalcontractsize"`
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
		HashType     string              `json:"hashtype"`
		Block        types.Block         `json:"block"`
		Blocks       []types.Block       `json:"blocks"`
		Transaction  types.Transaction   `json:"transaction"`
		Transactions []types.Transaction `json:"transactions"`
	}
)

// explorerHandlerGET handles GET requests to /explorer.
func (srv *Server) explorerHandlerGET(w http.ResponseWriter, req *http.Request) {
	stats := srv.explorer.Statistics()
	writeJSON(w, ExplorerGET{
		Height:            stats.Height,
		Block:             stats.Block,
		Target:            stats.Target,
		Difficulty:        stats.Difficulty,
		MaturityTimestamp: stats.MaturityTimestamp,
		Circulation:       stats.Circulation,

		TransactionCount:          stats.TransactionCount,
		SiacoinInputCount:         stats.SiacoinInputCount,
		SiacoinOutputCount:        stats.SiacoinOutputCount,
		FileContractCount:         stats.FileContractCount,
		FileContractRevisionCount: stats.FileContractRevisionCount,
		StorageProofCount:         stats.StorageProofCount,
		SiafundInputCount:         stats.SiafundInputCount,
		SiafundOutputCount:        stats.SiafundOutputCount,
		MinerFeeCount:             stats.MinerFeeCount,
		ArbitraryDataCount:        stats.ArbitraryDataCount,
		TransactionSignatureCount: stats.TransactionSignatureCount,

		ActiveContractCount: stats.ActiveContractCount,
		ActiveContractCost:  stats.ActiveContractCost,
		ActiveContractSize:  stats.ActiveContractSize,
		TotalContractCost:   stats.TotalContractCost,
		TotalContractSize:   stats.TotalContractSize,
	})
}

// explorerHandler handles API calls to /explorer.
func (srv *Server) explorerHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.explorerHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /explorer", http.StatusBadRequest)
}

// explorerHashHandlerGET handles GET requests to /explorer/hash.
func (srv *Server) explorerHashHandlerGET(w http.ResponseWriter, req *http.Request) {
	// Grab the hash in question.
	//
	// TODO: Might need to worry about checksums.
	hash := ???

	// Try the hash as each type of potential object.
	block, exists = srv.explorer.Block(types.BlockID(hash))
	if exists {
		writeJSON(w, ExplorerHashGET{
			HashType: "blockid",
			Block: block,
		})
		return
	}
	block, exists = srv.explorer.Transaction(types.TransactionID(hash))
	if exists {
		var txn types.Transaction
		for _, t := range block.Transactions {
			if t.ID() == types.TransactionID(hash) {
				txn = t
			}
		}
		writeJSON(w, ExplorerHashGET{
			HashType: "transactionid",
			Transaction: txn,
		})
		return
	}
	txids := srv.explorer.UnlockHash(types.UnlockHash(hash))
	if len(txids) != 0 {
		var txns []types.Transaction
		var blocks []types.Block
		for _, txid := range txids {
			block, exists := srv.explorer.Transaction(txid)
			if !exists && build.DEBUG {
				panic("explorer pointing to nonexistant txn")
			}
			if block.ID() == txid {
				blocks = append(blocks, block)
			}
			for _, t := range block.Transactions {
				if t.ID() == txid {
					txns = append(txns, t)
				}
			}
		}
		writeJSON(w, ExplorerHashGET{
			HashType: "unlockhash",
			Blocks: blocks,
			Transactions: txns,
		})
		return
	}
	txids := srv.explorer.SiacoinOutputID(types.SiacoinOutputID(hash))
	if len(txids) != 0 {
		var txns []types.Transaction
		var blocks []types.Block
		for _, txid := range txids {
			block, exists := srv.explorer.Transaction(txid)
			if !exists && build.DEBUG {
				panic("explorer pointing to nonexistant txn")
			}
			if block.ID() == txid {
				blocks = append(blocks, block)
			}
			for _, t := range block.Transactions {
				if t.ID() == txid {
					txns = append(txns, t)
				}
			}
		}
		writeJSON(w, ExplorerHashGET{
			HashType: "siacoinoutputid",
			Blocks: blocks,
			Transactions: txns,
		})
		return
	}
	txids := srv.explorer.FileContractID(types.FileContractID(hash))
	if len(txids) != 0 {
		var txns []types.Transaction
		var blocks []types.Block
		for _, txid := range txids {
			block, exists := srv.explorer.Transaction(txid)
			if !exists && build.DEBUG {
				panic("explorer pointing to nonexistant txn")
			}
			if block.ID() == txid {
				blocks = append(blocks, block)
			}
			for _, t := range block.Transactions {
				if t.ID() == txid {
					txns = append(txns, t)
				}
			}
		}
		writeJSON(w, ExplorerHashGET{
			HashType: "filecontractid",
			Blocks: blocks,
			Transactions: txns,
		})
		return
	}
	txids := srv.explorer.SiafundOutputID(types.SiafundOutputID(hash))
	if len(txids) != 0 {
		var txns []types.Transaction
		var blocks []types.Block
		for _, txid := range txids {
			block, exists := srv.explorer.Transaction(txid)
			if !exists && build.DEBUG {
				panic("explorer pointing to nonexistant txn")
			}
			if block.ID() == txid {
				blocks = append(blocks, block)
			}
			for _, t := range block.Transactions {
				if t.ID() == txid {
					txns = append(txns, t)
				}
			}
		}
		writeJSON(w, ExplorerHashGET{
			HashType: "siafundoutputid",
			Blocks: blocks,
			Transactions: txns,
		})
		return
	}

	// siacoin output ids
	// file contract ids
	// siafund output ids
}

// explorerHashHandler handles API calls to /explorer/hash.
func (srv *Server) explorerHashHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method == "" || req.Method == "GET" {
		srv.explorerHashHandlerGET(w, req)
		return
	}
	writeError(w, "unrecognized method when calling /explorer/hash", http.StatusBadRequest)
}
