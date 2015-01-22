package sia

import (
	"testing"
)

// sendManyTransactions repeatedly sends transactions, attempting to trigger a
// bug where transactions randomly fail to verify. Though this bug has since
// been found and fixed, the tests have been left behind.
func sendManyTransactions(t *testing.T, c *Core) {
	if testing.Short() {
		return
	}

	// Unfortunately, only 1 transaction per block can be sent, because the
	// wallet doesn't work with txn pool changes yet.
	blocks := 12
	for i := 0; i < blocks; i++ {
		address, _, err := c.wallet.CoinAddress()
		if err != nil {
			return
		}

		// Can only send as many transactions as we have inputs. Because we
		// send things to ourselves and then get the refund as well, we get a
		// new transaction we send, but the delay is one block.
		for j := 0; j < i; j++ {
			_, err := c.SpendCoins(123, address)
			if err != nil {
				t.Error(err)
			}
		}

		mineSingleBlock(t, c)
	}
}
