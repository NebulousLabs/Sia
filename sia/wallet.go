package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

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
func (c *Core) CoinAddress() (address consensus.CoinAddress, err error) {
	address, _, err = c.wallet.CoinAddress()
	return
}

// Returns a []byte that's supposed to be json of some struct.
func (c *Core) WalletInfo() (components.WalletInfo, error) {
	return c.wallet.WalletInfo()
}
