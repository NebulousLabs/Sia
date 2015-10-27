package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// ExplorerDir is the name of the directory that is typically used for the
	// explorer.
	ExplorerDir = "explorer"
)

type (
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

	// Explorer tracks the blockchain and provides tools for gathering
	// statistics and finding objects or patterns within the blockchain.
	Explorer interface {
		Statistics() ExplorerStatistics

		Close() error
	}
)
