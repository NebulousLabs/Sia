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
	CoinAddress() (consensus.UnlockHash, consensus.UnlockConditions, error)

	// TimelockedCoinAddress returns an address that can only be spent after block `unlockHeight`.
	TimelockedCoinAddress(unlockHeight consensus.BlockHeight) (consensus.UnlockHash, consensus.UnlockConditions, error)

	// RegisterTransaction creates a transaction out of an existing transaction
	// which can be modified by the wallet, returning an id that can be used to
	// reference the transaction.
	RegisterTransaction(consensus.Transaction) (id string, err error)

	// FundTransaction will add `amount` to a transaction's inputs. The funded
	// transaction is returned with an error.
	FundTransaction(id string, amount consensus.Currency) (consensus.Transaction, error)

	// AddMinerFee adds a single miner fee of value `fee`. The transaction is
	// returned, along with the index that the added fee ended up at.
	AddMinerFee(id string, fee consensus.Currency) (consensus.Transaction, uint64, error)

	// AddOutput adds an output to a transaction. It returns the transaction
	// with index of the output that got added.
	AddOutput(id string, output consensus.SiacoinOutput) (consensus.Transaction, uint64, error)

	// AddFileContract adds a file contract to a transaction, returning the
	// transaction and the index that the file contract was put at.
	AddFileContract(id string, fc consensus.FileContract) (consensus.Transaction, uint64, error)

	// AddStorageProof adds a storage proof to a transaction, returning the
	// transaction and the index that the storage proof was put at.
	AddStorageProof(id string, sp consensus.StorageProof) (consensus.Transaction, uint64, error)

	// AddArbitraryData adds a byte slice to the arbitrary data section of the
	// transaction, returning the transaction and the index of the new
	// arbitrary data.
	AddArbitraryData(id string, arb string) (consensus.Transaction, uint64, error)

	// AddSignature adds a signature to the transaction, the signature should
	// already be valid, and shouldn't sign any of the inputs that were added
	// by calling 'FundTransaction'. The updated transaction and the index of
	// the new signature are returned.
	AddSignature(id string, sig consensus.TransactionSignature) (consensus.Transaction, uint64, error)

	// Sign transaction will sign the transaction associated with the id and
	// then return the transaction. If wholeTransaction is set to true, then
	// the wholeTransaction flag will be set in CoveredFields for each
	// signature. After being signed, the transaction is deleted from the
	// wallet and must be reregistered if more changes are to be made.
	SignTransaction(id string, wholeTransaction bool) (consensus.Transaction, error)
}
