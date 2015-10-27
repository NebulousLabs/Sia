package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	ExplorerDir = "explorer"
)

// Used for the BlockInfo call
type (
	ExplorerBlockData struct {
		ID        types.BlockID   // The id hash of the block
		Timestamp types.Timestamp // The timestamp on the block
		Target    types.Target    // The target the block was mined for
		Size      uint64          // The size in bytes of the marshalled block
	}

	// ExplorerStatistics returns a bunch of instantaneous statistics about the
	// explorer at the current height.
	ExplorerStatistics struct {
		// General consensus information.
		Height            types.BlockHeight
		Block             types.BlockID
		Target            types.Target
		Difficulty        types.Currency
		MaturityTimestamp types.Timestamp
		Circulation       types.Currency

		// Information about transaction type usage.
		TransactionCount          uint64
		SiacoinInputCount         uint64
		SiacoinOutputCount        uint64
		FileContractCount         uint64
		FileContractRevisionCount uint64
		StorageProofCount         uint64
		SiafundInputCount         uint64
		SiafundOutputCount        uint64
		MinerFeeCount             uint64
		ArbitraryDataCount        uint64
		TransactionSignatureCount uint64

		// Information about file contracts and file contract revisions.
		ActiveContractCount uint64
		ActiveContractCost  types.Currency
		ActiveContractSize  types.Currency
		TotalContractCost   types.Currency
		TotalContractSize   types.Currency
	}

	// The following are used when returning information about a
	// hash (using the GetHashInfo function)
	//
	// The responseType field is used to differentiate the structs
	// blindly, and must be set
	BlockResponse struct {
		Block        types.Block
		Height       types.BlockHeight
		ResponseType string
	}

	// Wrapper for a transaction, with a little extra info
	TransactionResponse struct {
		Tx           types.Transaction
		ParentID     types.BlockID
		TxNum        int
		ResponseType string
	}

	// Wrapper for fcInfo struct, defined in database.go
	FcResponse struct {
		Contract     types.TransactionID
		Revisions    []types.TransactionID
		Proof        types.TransactionID
		ResponseType string
	}

	// Wrapper for the address type response
	AddrResponse struct {
		Txns         []types.TransactionID
		ResponseType string
	}

	OutputResponse struct {
		OutputTx     types.TransactionID
		InputTx      types.TransactionID
		ResponseType string
	}

	// The BlockExplorer interface provides access to the block explorer
	Explorer interface {
		BlockInfo(types.BlockHeight, types.BlockHeight) ([]ExplorerBlockData, error)

		Statistics() ExplorerStatistics

		Close() error

		GetHashInfo([]byte) (interface{}, error)
	}
)
