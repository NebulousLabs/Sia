package components

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type WalletInfo struct {
	Balance      consensus.Currency
	FullBalance  consensus.Currency
	NumAddresses int
}

// Wallet in an interface that helps to build and sign transactions. The user
// can make a new transaction-in-progress by calling Register(), and then can
// add outputs, fees, etc.
//
// TODO: CoinAddress returns spend conditions, add a TimelockedCoinAddress().
// This will obsolete the AddTimelockedRefund() function.
type Wallet interface {
	// Info takes zero arguments and returns an arbitrary set of information
	// about the wallet in the form of json. The frontend will have to know how
	// to parse it, but Core and Daemon don't need to understand what's in the
	// json.
	WalletInfo() (WalletInfo, error)

	// Update takes two sets of blocks. The first is the set of blocks that
	// have been rewound since the previous call to update, and the second set
	// is the blocks that were applied after rewinding.
	Update([]consensus.OutputDiff) error

	// Reset will clear the list of spent transactions, which is nice if you've
	// accidentally made transactions that aren't spreading on the network for
	// whatever reason (for example, 0 fee transaction, or if there are bugs in
	// the software). Reset will also destroy all in-progress transactions.
	Reset() error

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

	// AddOutput adds an output to a transaction.
	AddOutput(id string, output consensus.Output) error

	// AddTimelockedRefund will create an output with coins that are locked
	// until block `release`. The spend conditions are returned so that they
	// can be shown as proof that coins have been timelocked.
	//
	// It's a refund and not an output because currently the only way for a
	// wallet to know that it can spend a timelocked address is if the wallet
	// made the address itself.
	//
	// TODO: Eventually, there should be an extension that allows requests of
	// timelocked coin addresses.
	AddTimelockedRefund(id string, amount consensus.Currency, release consensus.BlockHeight) (sc consensus.SpendConditions, refundIndex uint64, err error)

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
