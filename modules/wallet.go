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
//
// DEPRECATED
type WalletInfo struct {
	Balance          types.Currency
	FullBalance      types.Currency
	VisibleAddresses []types.UnlockHash
	NumAddresses     int
}

// The TransactionBuilder is used to construct custom transactions. The general
// flow is to register a transaction, fund the transaction, add custom fields,
// and then sign the transaction. When signing a transaction, the only objects
// that get signatures are those added during the call 'FundTransaction'.
// Modifications are all additive.
//
// Transactions are tracked by an id, which is provided after registering a
// transaction. When manipulating or viewing a transaction, the id must be used
// to specify which transaction is being manipulated. The
// transaction-in-progress can be viewed at any time (in an incomplete,
// unsigned form) by calling 'ViewTransaction'.
//
// Transactions are kept with their parents. A parent is any unconfirmed
// transaction that is required for the child to be valid.
type TransactionBuilder interface {
	// RegisterTransaction takes a transaction and its parents returns an id
	// that can be used to modify the transaction. The most typical call is
	// 'RegisterTransaction(types.Transaction{}, nil)', which registers a new
	// transaction that doesn't have any parents. The id that gets returned is
	// not a types.TransactionID, it is an int and is only useful within the
	// transaction builder.
	RegisterTransaction(t types.Transaction, parents []types.Transaction) (id int, err error)

	// FundTransaction will create a transaction with a siacoin output
	// containing a value of exactly 'amount' - this prevents any refunds from
	// appearing in the primary transaction, but adds some number (usually one,
	// but can be more or less) of parent transactions. The parent transactions
	// are signed immediately, but the child transaction will not be signed
	// until 'SignTransaction' is called.
	FundTransaction(id int, amount types.Currency) error

	// AddMinerFee adds a single miner fee of value 'fee' to a transaction
	// specified by the registration id. The index of the fee within the
	// transaction is returned.
	AddMinerFee(id int, fee types.Currency) (uint64, error)

	// AddSiacoinInput adds a siacoin input to a transaction, specified by the
	// registration id.  When 'SignTransaction' gets called, this input will be
	// left unsigned.  The index of the siacoin input within the transaction is
	// returned.
	AddSiacoinInput(int, types.SiacoinInput) (uint64, error)

	// AddSiacoinOutput adds an output to a transaction, specified by id. The
	// index of the siacoin output within the transaction is returned.
	AddSiacoinOutput(int, types.SiacoinOutput) (uint64, error)

	// AddFileContract adds a file contract to a transaction, specified by id.
	// The index of the file contract within the transaction is returned.
	AddFileContract(int, types.FileContract) (uint64, error)

	// AddFileContractRevision adds a file contract revision to a transaction,
	// specified by id. The index of the file contract revision within the
	// transaction is returned.
	AddFileContractRevision(int, types.FileContractRevision) (uint64, error)

	// AddStorageProof adds a storage proof to a transaction, specified by id.
	// The index of the storage proof within the transaction is returned.
	AddStorageProof(int, types.StorageProof) (uint64, error)

	// AddSiafundInput adds a siacoin input to the transaction, specified by
	// id. When 'SignTransaction' gets called, this input will be left
	// unsigned. The index of the siafund input within the transaction is
	// returned.
	AddSiafundInput(int, types.SiafundInput) (uint64, error)

	// AddSiafundOutput adds an output to a transaction, specified by
	// registration id. The index of the siafund output within the transaction
	// is returned.
	AddSiafundOutput(int, types.SiafundOutput) (uint64, error)

	// AddArbitraryData adds a byte slice to the arbitrary data section of the
	// transaction. The index of the arbitrary data within the transaction is
	// returned.
	AddArbitraryData(id int, arb []byte) (uint64, error)

	// AddTransactionSignature adds a signature to the transaction, the
	// signature should already be valid, and shouldn't sign any of the inputs
	// that were added by calling 'FundTransaction'. The updated transaction
	// and the index of the new signature are returned.
	AddTransactionSignature(int, types.TransactionSignature) (uint64, error)

	// SignTransaction will sign and delete a transaction, specified by
	// registration id. If the whole transaction flag is set to true, then the
	// covered fields object in each of the transaction signatures will have
	// the whole transaction field set. Otherwise, the flag will not be set but
	// the signature will cover all known fields in the transaction (see an
	// implementation for more clarity). After signing, a transaction set will
	// be returned that contains all parents followed by the transaction. The
	// transaction is then deleted from the builder registry.
	SignTransaction(id int, wholeTransaction bool) (txnSet []types.Transaction, err error)

	// ViewTransaction returns a transaction-in-progress along with all of its
	// parents, specified by id. An error is returned if the id is invalid.
	// Note that ids become invalid for a transaction after 'SignTransaction'
	// has been called because the transaction gets deleted.
	ViewTransaction(id int) (txn types.Transaction, parents []types.Transaction, err error)
}

// Wallet is an interface that keeps track of addresses and balance. Using
// TransactionBuilder it also streamlines sending coins.
type Wallet interface {
	TransactionBuilder

	// Balance returns the total number of coins accessible to the wallet. If
	// full == true, the number of coins returned will also include coins that
	// have been spent in unconfirmed transactions.
	Balance(full bool) types.Currency

	// Close safely closes the wallet file.
	Close() error

	// CoinAddress return an address into which coins can be paid. The bool
	// indicates whether the address should be visible to the user.
	CoinAddress(visible bool) (types.UnlockHash, types.UnlockConditions, error)

	Info() WalletInfo

	// MergeWallet takes a filepath to another wallet that should be merged
	// with the current wallet. Repeat addresses will not be merged.
	MergeWallet(string) error

	// TimelockedCoinAddress returns an address that can only be spent after
	// block `unlockHeight`. The bool indicates whether the address should be
	// visible to the user.
	TimelockedCoinAddress(unlockHeight types.BlockHeight, visible bool) (types.UnlockHash, types.UnlockConditions, error)

	SendCoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error)

	// SiafundBalance returns the number of siafunds owned by the wallet, and
	// the number of siacoins available through siafund claims.
	SiafundBalance() (siafundBalance types.Currency, siacoinClaimBalance types.Currency)

	// SendSiagSiafunds sends siafunds to another address. The siacoins stored
	// in the siafunds are sent to an address in the wallet.
	SendSiagSiafunds(amount types.Currency, dest types.UnlockHash, keyfiles []string) ([]types.Transaction, error)

	// WatchSiagSiafundAddress adds a siafund address pulled from a siag keyfile.
	WatchSiagSiafundAddress(keyfile string) error
}
