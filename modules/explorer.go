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
	// ExplorerStatistics returns a bunch of statistics about the explorer at
	// the current height.
	ExplorerStatistics struct {
		// General consensus information.
		Height            types.BlockHeight
		CurrentBlock      types.BlockID
		Target            types.Target
		Difficulty        types.Currency
		MaturityTimestamp types.Timestamp
		TotalCoins        types.Currency

		// Information about transaction type usage.
		MinerPayoutCount          uint64
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

	// BlockFacts returns a bunch of statistics about the consensus set asa
	// they were at a specific block.
	BlockFacts struct {
		BlockID types.BlockID
		Height  types.BlockHeight

		// Transaction type counts.
		MinerPayoutCount          uint64
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

		// Factoids about file contracts.
		ActiveContractCost  types.Currency
		ActiveContractCount uint64
		ActiveContractSize  types.Currency
		TotalContractCost   types.Currency
		TotalContractSize   types.Currency
		TotalRevisionVolume types.Currency
	}

	// Explorer tracks the blockchain and provides tools for gathering
	// statistics and finding objects or patterns within the blockchain.
	Explorer interface {
		// Statistics provides general statistics about the blockchain.
		Statistics() ExplorerStatistics

		// Block returns the block that matches the input block id. The bool
		// indicates whether the block appears in the blockchain.
		Block(types.BlockID) (types.Block, types.BlockHeight, bool)

		// BlockFacts returns a set of statistics about the blockchain as they
		// appeared at a given block.
		BlockFacts(types.BlockHeight) (BlockFacts, bool)

		// Transaction returns the block that contains the input transaction
		// id. The transaction itself is either the block (indicating the miner
		// payouts are somehow involved), or it is a transaction inside of the
		// block. The bool indicates whether the transaction is found in the
		// consensus set.
		Transaction(types.TransactionID) (types.Block, types.BlockHeight, bool)

		// UnlockHash returns all of the transaction ids associated with the
		// provided unlock hash.
		UnlockHash(types.UnlockHash) []types.TransactionID

		// SiacoinOutputID returns all of the transaction ids associated with
		// the provided siacoin output id.
		SiacoinOutputID(types.SiacoinOutputID) []types.TransactionID

		// FileContractID returns all of the transaction ids associated with
		// the provided file contract id.
		FileContractID(types.FileContractID) []types.TransactionID

		// SiafundOutputID returns all of the transaction ids associated with
		// the provided siafund output id.
		SiafundOutputID(types.SiafundOutputID) []types.TransactionID

		Close() error
	}
)
