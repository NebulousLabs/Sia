package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// Wallet in an interface that helps to build and sign transactions. The user
// can make a new transaction-in-progress by calling Register(), and then can
// add outputs, fees, etc.
type Wallet interface {
	// Info takes zero arguments and returns an arbitrary set of information
	// about the wallet in the form of json. The frontend will have to know how
	// to parse it, but Core and Daemon don't need to understand what's in the
	// json.
	Info() ([]byte, error)

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
	CoinAddress() (consensus.CoinAddress, error)

	// RegisterTransaction creates a transaction out of an existing transaction
	// which can be modified by the wallet, returning an id that can be used to
	// reference the transaction.
	RegisterTransaction(consensus.Transaction) (id string, err error)

	// FundTransaction will add `amount` to a transaction's inputs.
	FundTransaction(id string, amount consensus.Currency) error

	// AddMinerFee adds a single miner fee of value `fee`.
	AddMinerFee(id string, fee consensus.Currency) error

	// AddOutput adds an output of value `amount` to address `ca`.
	AddOutput(id string, o consensus.Output) error

	// AddTimelockedOutput will create an output with coins that are locked
	// until block `release`. The spend conditions are returned so that they
	// can be shown as proof that coins have been timelocked.
	AddTimelockedOutput(id string, amount consensus.Currency, dest consensus.CoinAddress, release consensus.BlockHeight) (sc consensus.SpendConditions, refundIndex uint64, err error)

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

// SpendCoins creates a transaction sending 'amount' to 'dest', and
// allocateding 'minerFee' as a miner fee. The transaction is submitted to the
// miner pool, but is also returned.
func (c *Core) SpendCoins(amount consensus.Currency, dest consensus.CoinAddress) (t consensus.Transaction, err error) {
	// Create and send the transaction.
	minerFee := consensus.Currency(10) // TODO: wallet supplied miner fee
	output := consensus.Output{
		Value:     amount,
		SpendHash: dest,
	}
	id, err := c.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = c.wallet.FundTransaction(id, amount+minerFee)
	if err != nil {
		return
	}
	err = c.wallet.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = c.wallet.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = c.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = c.AcceptTransaction(t)
	return
}

// WalletBalance counts up the total number of coins that the wallet knows how
// to spend, according to the State. WalletBalance will ignore all unconfirmed
// transactions that have been created.
func (c *Core) WalletBalance(full bool) consensus.Currency {
	return c.wallet.Balance(full)
}

// CoinAddress returns the CoinAddress which foreign coins should
// be sent to.
func (c *Core) CoinAddress() (consensus.CoinAddress, error) {
	return c.wallet.CoinAddress()
}

// Returns a []byte that's supposed to be json of some struct.
func (c *Core) WalletInfo() ([]byte, error) {
	return c.wallet.Info()
}
