package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

const (
	WalletDir = "wallet"
)

var (
	LowBalanceErr = errors.New("Insufficient Balance")
)

// WalletInfo contains basic information about the wallet.
type WalletInfo struct {
	Balance          types.Currency
	FullBalance      types.Currency
	VisibleAddresses []types.UnlockHash
	NumAddresses     int
}

// Wallet is an interface that keeps track of addresses and balance. Using TransactionBuilder
// it also streamlines sending coins.
type Wallet interface {
	TransactionBuilder
	// Balance returns the total number of coins accessible to the wallet. If
	// full == true, the number of coins returned will also include coins that
	// have been spent in unconfirmed transactions.
	Balance(full bool) types.Currency

	// CoinAddress return an address into which coins can be paid. The bool
	// indicates whether the address should be visible to the user.
	CoinAddress(visible bool) (types.UnlockHash, types.UnlockConditions, error)

	// TimelockedCoinAddress returns an address that can only be spent after
	// block `unlockHeight`. The bool indicates whether the address should be
	// visible to the user.
	TimelockedCoinAddress(unlockHeight types.BlockHeight, visible bool) (types.UnlockHash, types.UnlockConditions, error)

	Info() WalletInfo

	SpendCoins(amount types.Currency, dest types.UnlockHash) (types.Transaction, error)

	// WalletNotify will push a struct down the channel any time that the
	// wallet updates.
	WalletNotify() <-chan struct{}

	// Close safely closes the wallet file.
	Close() error
}

// TransactionBuilder is an interface in which transactions are registered, detailed, and signed.
// This gives other modules, and wallet itself, full flexibility in creating dynamic transactions.
type TransactionBuilder interface {
	// RegisterTransaction creates a transaction out of an existing transaction
	// which can be modified by the wallet, returning an id that can be used to
	// reference the transaction.
	RegisterTransaction(types.Transaction) (id string, err error)

	// FundTransaction will add `amount` to a transaction's inputs. The funded
	// transaction is returned with an error.
	FundTransaction(id string, amount types.Currency) (types.Transaction, error)

	// AddSiacoinInput adds a siacoin input to the transaction. When
	// 'SignTransaction' gets called, this input will be left unsigned. The
	// updated transaction is returned along with the index of the new siacoin
	// input within the transaction.
	AddSiacoinInput(id string, input types.SiacoinInput) (types.Transaction, uint64, error)

	// AddMinerFee adds a single miner fee of value `fee`. The transaction is
	// returned, along with the index that the added fee ended up at.
	AddMinerFee(id string, fee types.Currency) (types.Transaction, uint64, error)

	// AddOutput adds an output to a transaction. It returns the transaction
	// with index of the output that got added.
	AddOutput(id string, output types.SiacoinOutput) (types.Transaction, uint64, error)

	// AddFileContract adds a file contract to a transaction, returning the
	// transaction and the index that the file contract was put at.
	AddFileContract(id string, fc types.FileContract) (types.Transaction, uint64, error)

	// AddStorageProof adds a storage proof to a transaction, returning the
	// transaction and the index that the storage proof was put at.
	AddStorageProof(id string, sp types.StorageProof) (types.Transaction, uint64, error)

	// AddArbitraryData adds a byte slice to the arbitrary data section of the
	// transaction, returning the transaction and the index of the new
	// arbitrary data.
	AddArbitraryData(id string, arb string) (types.Transaction, uint64, error)

	// AddSignature adds a signature to the transaction, the signature should
	// already be valid, and shouldn't sign any of the inputs that were added
	// by calling 'FundTransaction'. The updated transaction and the index of
	// the new signature are returned.
	AddSignature(id string, sig types.TransactionSignature) (types.Transaction, uint64, error)

	// Sign transaction will sign the transaction associated with the id and
	// then return the transaction. If wholeTransaction is set to true, then
	// the wholeTransaction flag will be set in CoveredFields for each
	// signature. After being signed, the transaction is deleted from the
	// wallet and must be reregistered if more changes are to be made.
	SignTransaction(id string, wholeTransaction bool) (types.Transaction, error)
}
