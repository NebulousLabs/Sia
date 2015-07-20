package modules

import (
	"github.com/NebulousLabs/Sia/crypto"
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

	ExplorerStatus struct {
		Height              types.BlockHeight
		Block               types.Block
		Target              types.Target
		MatureTime          types.Timestamp
		TotalCurrency       types.Currency
		ActiveContractCount uint64
		ActiveContractCosts types.Currency
		ActiveContractSize  uint64
		TotalContractCount  uint64
		TotalContractCosts  types.Currency
		TotalContractSize   uint64
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
		Contract     crypto.Hash
		Revisions    []crypto.Hash
		Proof        crypto.Hash
		ResponseType string
	}

	// Wrapper for the address type response
	AddrResponse struct {
		Txns         []crypto.Hash
		ResponseType string
	}

	OutputResponse struct {
		OutputTx     crypto.Hash
		InputTx      crypto.Hash
		ResponseType string
	}

	// The BlockExplorer interface provides access to the block explorer
	Explorer interface {
		// Returns a slice of data points about blocks. Called
		// primarly by the blockdata api call
		BlockInfo(types.BlockHeight, types.BlockHeight) ([]ExplorerBlockData, error)

		// Function to return status of a bunch of static variables,
		// in the form of an ExplorerStatus struct
		ExplorerStatus() ExplorerStatus

		// Function to safely shut down the block explorer. Closes the database
		Close() error

		// Returns information pertaining to a given hash. The
		// type of the returned value depends on what the hash
		// was, so an interface is returned instead (i.e. an
		// address will return a list of transactions while a
		// block ID will return a block
		GetHashInfo([]byte) (interface{}, error)
	}
)
