package modules

import (
	"errors"

	"github.com/NebulousLabs/Sia/types"
)

const (
	WalletDir = "wallet"

	PublicKeysPerSeed = 100
)

var (
	LowBalanceErr = errors.New("Insufficient Balance")
)

// AddressSeed is cryptographic entropy that is used to derive spendable wallet
// addresses.
type Seed [crypto.EntropySize]byte

// WalletTransactionID is a unique identifier for a wallet transaction.
type WalletTransactionID crypto.Hash

// WalletTransaction contains the metadata of a single output that changed the
// balance of the wallet, either incoming or outgoing (which can be gleaned
// from the 'Source' and 'Destination'.
type WalletTransaction struct {
	TransactionID types.TransactionID
	ConfirmationHeight types.BlockHeight
	ConfirmationTimestamp types.Timestamp
	Transaction types.Transaction

	FundType types.Specifier
	OutputID OutputID
	RelatedAddress types.UnlockHash
	Value types.Currency
}

// TransactionBuilder is used to construct custom transactions. A transaction
// builder is intialized via 'RegisterTransaction' and then can be modified by
// adding funds or other fields. The transaction is completed by calling
// 'Sign', which will sign all inputs added via the 'FundSiacoins' or
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
	// called on the transaction builder. The expectation is that the
	// transaction will be completed and broadcast within a few hours. Longer
	// risks double-spends, as the wallet will assume that the transaction
	// failed.
	FundSiacoins(amount types.Currency) error

	// FundSiafunds will add a siafund input of exaclty 'amount' to the
	// transaction. A parent transaction may be needed to achieve an input with
	// the correct value. The siafund input will not be signed until 'Sign' is
	// called on the transaction builder. Any siacoins that are released by
	// spending the siafund outputs will be sent to another address owned by
	// the wallet. The expectation is that the transaction will be completed
	// and broadcast within a few hours. Longer risks double-spends, because
	// the wallet will assume the transcation failed.
	FundSiafunds(amount types.Currency) error

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

// Wallet stores and manages siacoins and siafunds. The wallet file is
// encrypted using a user-specified password. Common addresses are all dervied
// from a single address seed.
type Wallet interface {
	// Encrypted returns whether or not the wallet has been encrypted yet. User
	// facings apps are recommended to check if the wallet is encrypted before
	// calling Unlock, because the key used in the first call to 'Unlock' will
	// be the key that encrypts the wallet going forward. User facing apps
	// should verify that the correct password/phrase/key was chosen before
	// permanently encrypting the wallet.
	Encrypted() bool

	// Unlock must be called before the wallet is usable. All wallets and
	// wallet seeds are encrypted by default, and the wallet will not know
	// which addresses to watch for on the blockchain until unlock has been
	// called.
	//
	// All items in the wallet are encrypted using different keys which are
	// derived from the master key.
	Unlock(masterKey crypto.TwofishKey) error

	// NewPrimarySeed will generate a new primary seed from which addresses
	// will be derived. Each seed can produce up to 'PublicKeysPerSeed' seeds,
	// after which an error will be returned when requesting new addresses. The
	// string returned is the recovery string for the seed. If the wallet file
	// is lost, the recovery string may be used to regain the files. The master
	// key is used to encrypt the seed when saving it to disk. The secret keys
	// for the primary seed are kept unencrypted in memory.
	NewPrimarySeed(masterKey crypto.TwofishKey) (Seed, error)

	// PrimarySeed returns the current primary seed of the wallet, unencrypted,
	// with an int indicating how many addresses have been consumed out of
	// 'PublicKeysPerSeed' total addresses.
	PrimarySeed() (Seed, int, error)

	// RecoverSeed will recreate a wallet file using the recovery phrase.
	// RecoverSeed only needs to be called if the original seed file or
	// encryption password was lost. The master key is used encrypt the
	// recovery seed before saving it to disk.
	RecoverSeed(masterKey crypto.TwofishKey, Seed) error

	// AllSeeds returns all of the seeds that are being tracked by the wallet,
	// including the primary seed. Only the primary seed is used to generate
	// new addresses, but the wallet can spend funds sent to public keys
	// generated by any of the seeds returned.
	AllSeeds() ([]Seed, error)

	// ConfirmedBalance returns the confirmed balance of the wallet, minus any
	// outgoing transactions. ConfirmedBalance will include unconfirmed refund
	// transacitons.
	ConfirmedBalance() (siacoinBalance types.Currency, siafundBalance types.Currency, siacoinClaimBalance types.Currency)

	// UnconfirmedBalance returns the unconfirmed balance of the wallet.
	// Outgoing funds and incoming funds are reported separately. Refund
	// outputs are included, meaning that a sending a single coin to someone
	// could result in 'outgoing: 12, incoming: 11'. Siafunds are not
	// considered in the unconfirmed balance.
	UnconfirmedBalance() (outgoingSiacoins types.Currency, incomingSiacoins types.Currency)

	// TransacitonHistory will return a chronologically ordered set of
	// 'WalletTransactions' that make up the history of the wallet.
	TransactionHistory() []WalletTransaction

	// PartialTransactionHistory returns all of the transactions that were
	// confirmed at heights [startBlock, endBlock].
	PartialTransactionHistory(startBlock types.BlockHeight, endBlock types.BlockHeight) ([]WalletTransaction, error)

	// AddressTransactionHistory returns all of the transactions that are
	// related to a given address.
	AddressTransactionHistory(types.UnlockHash) []WalletTransaction

	// UnconfirmedTransactions returns the list of known unconfirmed wallet
	// transactions.
	UnconfirmedTransactions() []WalletTransaction

	// AddressUnconfirmedTransactions returns all of the wallet transactions
	// related to a given address.
	AddressUnconfirmedTransactions(types.UnlockHash) []WalletTransaction

	// CoinAddress returns an address that can receive coins.
	CoinAddress() (types.UnlockConditions, types.UnlockHash, error)

	// RegisterTransaction takes a transaction and its parents and returns a
	// TransactionBuilder which can be used to expand the transaction. The most
	// typical call is 'RegisterTransaction(types.Transaction{}, nil)', which
	// registers a new transaction without parents.
	RegisterTransaction(t types.Transaction, parents []types.Transaction) TransactionBuilder

	// StartTransaction is a convenience method that calls
	// RegisterTransaction(types.Transaction{}, nil)
	StartTransaction() TransactionBuilder

	// SendSiacoins is a tool for sending siacoins from the wallet to an
	// address. Sending money usually results in multiple transactions. The
	// transactions are automatically given to the transaction pool, and are
	// also returned to the caller.
	SendSiacoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error)

	// SendSiafunds is a tool for sending siafunds from the wallet to an
	// address. Sending money usually results in multiple transactions. The
	// transactions are automatically given to the transaction pool, and are
	// also returned to the caller.
	SendSiafunds(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error)

	// SendSiagSiafunds sends siafunds to another address. The siacoins stored
	// in the siafunds are sent to an address in the wallet.
	SendSiagSiafunds(amount types.Currency, dest types.UnlockHash, keyfiles []string) ([]types.Transaction, error)

	// WatchSiagSiafundAddress adds a siafund address pulled from a siag keyfile.
	WatchSiagSiafundAddress(keyfile string) error

	// Close prepares the wallet for shutdown.
	Close() error
}

// CalculateWalletTransactionID is a helper function for determining the id of
// a wallet transaction.
func CalcualteWalletTransactionID(tid types.TransactionID, oid OutputID) WalletTransactionID {
	return WalletTransactionID(crypto.HashAll(t, oid))
}
