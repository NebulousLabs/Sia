package proto

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// dependencies
type (
	transactionBuilder interface {
		AddFileContract(types.FileContract) uint64
		AddMinerFee(types.Currency) uint64
		AddParents([]types.Transaction)
		AddSiacoinInput(types.SiacoinInput) uint64
		AddSiacoinOutput(types.SiacoinOutput) uint64
		AddTransactionSignature(types.TransactionSignature) uint64
		FundSiacoins(types.Currency) error
		Sign(bool) ([]types.Transaction, error)
		View() (types.Transaction, []types.Transaction)
		ViewAdded() (parents, coins, funds, signatures []int)
	}

	transactionPool interface {
		AcceptTransactionSet([]types.Transaction) error
		FeeEstimation() (min types.Currency, max types.Currency)
	}
)

// A Contract contains all the metadata necessary to revise or renew a file
// contract.
type Contract struct {
	IP              modules.NetAddress
	ID              types.FileContractID
	FileContract    types.FileContract
	MerkleRoots     []crypto.Hash
	LastRevision    types.FileContractRevision
	LastRevisionTxn types.Transaction
	SecretKey       crypto.SecretKey
}

// ContractParams are supplied as an argument to FormContract.
type ContractParams struct {
	Host          modules.HostDBEntry
	Filesize      uint64
	StartHeight   types.BlockHeight
	EndHeight     types.BlockHeight
	RefundAddress types.UnlockHash
	// TODO: add optional keypair
}
