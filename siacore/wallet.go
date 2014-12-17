package siacore

import (
	"fmt"
	"io/ioutil"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/signatures"
)

// Wallet in an interface that helps to build and sign transactions.
// Transactions are kept in wallet memory until they are signed, and referenced
// using a string id.
//
// TODO: Reconsider how save, load, and reset work.
type Wallet interface {
	// Update takes two sets of blocks. The first is the set of blocks that
	// have been rewound since the previous call to update, and the second set
	// is the blocks that were applied after rewinding.
	Update(rewound []consensus.Block, applied []consensus.Block) error

	// Reset will clear the list of spent transactions, which is nice if you've
	// accidentally made transactions that aren't spreading on the network for
	// whatever reason (for example, 0 fee transaction, or if there are bugs in
	// the software). Conditions for reset are subject to change.
	Reset() error

	// Balance returns the total number of coins accessible to the wallet. If
	// full == true, the number of coins returned will also include coins that
	// have been spent in unconfirmed transactions.
	Balance(full bool) (consensus.Currency, error)

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

	// AddTimelockedRefund will add `amount` of coins to a transaction that
	// unlock at block `release`. The spend conditions of the output are
	// returned so that they can be revealed to interested parties. The coins
	// will be added back into the balance when the timelock expires.
	AddTimelockedRefund(id string, amount consensus.Currency, release consensus.BlockHeight) (consensus.SpendConditions, error)

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
	SignTransaction(id string, wholeTransaction bool) (consensus.Transaction, error)

	// Save creates a binary file containing keys and such so the coins
	// can be spent later.
	Save(filename string) error
}

// SpendCoins creates a transaction sending 'amount' to 'dest', and
// allocateding 'minerFee' as a miner fee. The transaction is submitted to the
// miner pool, but is also returned.
func (e *Environment) SpendCoins(amount, minerFee consensus.Currency, dest consensus.CoinAddress) (t consensus.Transaction, err error) {
	// Scan blockchain for outputs.
	e.wallet.Scan()

	// Add `amount` + `minerFee` coins to the transaction.
	err = e.wallet.FundTransaction(amount+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output to `dest`.
	t.Outputs = append(t.Outputs, consensus.Output{Value: amount, SpendHash: dest})

	// Sign each input.
	for i := range t.Inputs {
		err = e.wallet.SignTransaction(&t, consensus.CoveredFields{WholeTransaction: true}, i)
		if err != nil {
			return
		}
	}

	// Send the transaction to the environment.
	e.AcceptTransaction(t)

	return
}

// WalletBalance counts up the total number of coins that the wallet knows how
// to spend, according to the State. WalletBalance will ignore all unconfirmed
// transactions that have been created.
func (e *Environment) WalletBalance() (consensus.Currency, error) {
	return e.wallet.Balance()
}

// Environment.CoinAddress returns the CoinAddress which foreign coins should
// be sent to.
func (e *Environment) CoinAddress() consensus.CoinAddress {
	return e.wallet.SpendConditions.CoinAddress()
}
