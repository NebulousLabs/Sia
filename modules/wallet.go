package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
)

var (
	LowBalanceErr = errors.New("Insufficient Balance")
)

// Wallet in an interface that helps to build and sign transactions. The user
// can make a new transaction-in-progress by calling Register, and then can
// add outputs, fees, etc. This gives other modules full flexibility in
// creating dynamic transactions.
type Wallet interface {
	// Balance returns the total number of coins accessible to the wallet. If
	// full == true, the number of coins returned will also include coins that
	// have been spent in unconfirmed transactions.
	Balance(full bool) consensus.Currency

	// CoinAddress return an address into which coins can be paid.
	CoinAddress() (consensus.CoinAddress, consensus.SpendConditions, error)

	// TimelockedCoinAddress returns an address that can only be spent after block `unlockHeight`.
	TimelockedCoinAddress(unlockHeight consensus.BlockHeight) (consensus.CoinAddress, consensus.SpendConditions, error)

	// RegisterTransaction creates a transaction out of an existing transaction
	// which can be modified by the wallet, returning an id that can be used to
	// reference the transaction.
	RegisterTransaction(consensus.Transaction) (id string, err error)

	// FundTransaction will add `amount` to a transaction's inputs.
	FundTransaction(id string, amount consensus.Currency) error

	// AddMinerFee adds a single miner fee of value `fee`.
	AddMinerFee(id string, fee consensus.Currency) error

	// AddOutput adds an output to a transaction. It returns the index of the
	// output in the transaction.
	AddOutput(id string, output consensus.SiacoinOutput) (uint64, error)

	// AddFileContract adds a file contract to a transaction.
	AddFileContract(id string, fc consensus.FileContract) error

	// AddStorageProof adds a storage proof to a transaction.
	AddStorageProof(id string, sp consensus.StorageProof) error

	// AddArbitraryData adds a byte slice to the arbitrary data section of the
	// transaction.
	AddArbitraryData(id string, arb string) error

	// Sign transaction will sign the transaction associated with the id and
	// then return the transaction. If wholeTransaction is set to true, then
	// the wholeTransaction flag will be set in CoveredFields for each
	// signature.
	//
	// Upon being signed and returned, the transaction-in-progress is deleted
	// from the wallet.
	SignTransaction(id string, wholeTransaction bool) (consensus.Transaction, error)
}
