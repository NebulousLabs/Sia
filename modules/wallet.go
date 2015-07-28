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

// The TransactionBuilder is used to construct custom transactions. A
// transaction builder is intialized via 'RegisterTransaction' and then can be
// modified by adding funds or other fields. The transaction is completed by
// calling 'Sign', which will sign all inputs added via the 'FundSiacoins' or
// 'FundSiafunds' call. All modifications are additive.
//
// Parents of the transaction are kept in the transaction builder. A parent is
// any unconfirmed transaction that is required for the child to be valid.
//
// Transaction builders are not thread safe.
type TransactionBuilder interface {
	// FundSiacoins will add a siacoin input of exaclty 'amount' to the
	// transaction. A parent transaction may be needed to achieve an input with
	// the correct value. The siacoin input will not be signed until 'Sign' is
	// called on the transaction builder.
	FundSiacoins(amount types.Currency) error

	// TODO: Add a function 'FundSiafunds' - maybe best to wait for a clear use
	// case, but maybe not - the builder is supposed to be general.

	// AddMinerFee adds a miner fee to the transaction, returning the index of
	// the miner fee within the transaction.
	AddMinerFee(fee types.Currency) uint64

	// AddSiacoinInput adds a siacoin input to the transaction, returning the
	// index of the siacoin input within the transaction. When 'Sign' gets
	// called, this input will be left unsigned.
	AddSiacoinInput(types.SiacoinInput) uint64

	// AddSiacoinOutput adds a siacoin output to the transaction, returning the
	// index of the siacoin output within the transaction.
	AddSiacoinOutput(types.SiacoinOutput) uint64

	// AddFileContract adds a file contract to the transaction, returning the
	// index of the file contract within the transaction.
	AddFileContract(types.FileContract) uint64

	// AddFileContractRevision adds a file contract revision to the
	// transaction, returning the index of the file contract revision within
	// the transaction. When 'Sign' gets called, this revision will be left
	// unsigned.
	AddFileContractRevision(types.FileContractRevision) uint64

	// AddStorageProof adds a storage proof to the transaction, returning the
	// index of the storage proof within the transaction.
	AddStorageProof(types.StorageProof) uint64

	// AddSiafundInput adds a siafund input to the transaction, returning the
	// index of the siafund input within the transaction. When 'Sign' is
	// called, this input will be left unsigned.
	AddSiafundInput(types.SiafundInput) uint64

	// AddSiafundOutput adds a siafund output to the transaction, returning the
	// index of the siafund output within the transaction.
	AddSiafundOutput(types.SiafundOutput) uint64

	// AddArbitraryData adds arbitrary data to the transaction, returning the
	// index of the data within the transaction.
	AddArbitraryData(arb []byte) uint64

	// AddTransactionSignature adds a transaction signature to the transaction,
	// returning the index of the signature within the transaction. The
	// signature should already be valid, and shouldn't sign any of the inputs
	// that were added by calling 'FundSiacoins' or 'FundSiafunds'.
	AddTransactionSignature(types.TransactionSignature) uint64

	// Sign will sign any inputs added by 'FundSiacoins' or 'FundSiafunds' and
	// return a transaction set that contains all parents prepended to the
	// transaction. If more fields need to be added, a new transaction builder
	// will need to be created.
	//
	// If the whole transaction flag  is set to true, then the whole
	// transaction flag will be set in the covered fields object. If the whole
	// transaction flag is set to false, then the covered fields object will
	// cover all fields that have already been added to the transaction, but
	// will also leave room for more fields to be added.
	Sign(wholeTransaction bool) ([]types.Transaction, error)

	// View returns the incomplete transaction along with all of its parents.
	View() (txn types.Transaction, parents []types.Transaction)
}

type Wallet interface {
	// RegisterTransaction takes a transaction and its parents and returns a
	// TransactionBuilder which can be used to expand the transaction. The most
	// typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
	// registers a new transaction without parents.
	RegisterTransaction(t types.Transaction, parents []types.Transaction) TransactionBuilder

	// StartTransaction is a convenience method that calls
	// RegisterTransaction(types.Transaction{}, nil)
	StartTransaction() TransactionBuilder

	Balance(full bool) types.Currency

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

	// Unlock
	// NewSeed
	// ListSeeds
	// NextAddress
}
