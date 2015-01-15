package sia

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// sendManyTransactions was created becuase occasionally transaction sending
// would fail, returning an invalid signature error. This sent many simple
// transactions to trigger the seemingly random error. This doesn't actually
// test anything, it just does a bunch of actions and sees if an error
// triggers.
func sendManyTransactions(t *testing.T, c *Core) {
	if testing.Short() {
		return
	}

	// Unfortunately, only 1 transaction per block can be sent, because the
	// wallet doesn't work with txn pool changes yet.
	blocks := 20
	for i := 0; i < blocks; i++ {
		address, _, err := c.wallet.CoinAddress()
		if err != nil {
			return
		}

		// Can only send as many transactions as we have inputs. Because we
		// send things to ourselves and then get the refund as well, we get a
		// new transaction we send, but the delay is one block.
		for j := 0; j < i; j++ {
			txn, err := c.SpendCoins(123, address)
			if err != nil {
				t.Error(err)
			}
			err = c.processTransaction(txn)
			if err != nil && err != consensus.ConflictingTransactionErr {
				t.Error(err)
			}
		}

		mineSingleBlock(t, c)
	}
}
