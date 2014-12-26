package sia

import (
	"fmt"

	"github.com/NebulousLabs/Sia/consensus"
)

// BlockChan returns a channel down which blocks can be thrown.
func (c *Core) BlockChan() chan consensus.Block {
	return c.blockChan
}

func (c *Core) TransactionChan() chan consensus.Transaction {
	return c.transactionChan
}

// AcceptBlock sends the input block down a channel, where it will be dealt
// with by the Core's listener.
func (c *Core) AcceptBlock(b consensus.Block) error {
	c.blockChan <- b
	return nil
}

// AcceptTransaction sends the input transaction down a channel, where it will
// be dealt with by the Core's listener.
func (c *Core) AcceptTransaction(t consensus.Transaction) error {
	c.transactionChan <- t
	return nil
}

// processBlock is called by the environment's listener.
func (c *Core) processBlock(b consensus.Block) (err error) {
	c.state.Lock()
	// c.hostDatabase.Lock()
	// c.host.Lock()
	defer c.state.Unlock()
	// defer c.hostDatabase.Unlock()
	// defer c.host.Unlock()

	// initialStateHeight := c.state.Height()
	_, _, outputDiffs, err := c.state.AcceptBlock(b)
	if err == consensus.BlockKnownErr || err == consensus.KnownOrphanErr {
		return
	} else if err != nil {
		// Call CatchUp() if an unknown orphan is sent.
		if err == consensus.UnknownOrphanErr {
			go c.CatchUp(c.server.RandomPeer())
		}
		return
	}

	err = c.wallet.Update(outputDiffs)
	if err != nil {
		return
	}
	err = c.updateMiner(c.miner)
	if err != nil {
		return
	}
	// c.updateHostDB(rewoundBlockIDs, appliedBlockIDs)
	// c.storageProofMaintenance(initialStateHeight, rewoundBlockIDs, appliedBlockIDs)

	// Broadcast all valid blocks.
	go c.server.Broadcast("AcceptBlock", b, nil)
	return
}

// processTransaction sends a transaction to the state.
func (c *Core) processTransaction(t consensus.Transaction) (err error) {
	c.state.Lock()
	defer c.state.Unlock()

	err = c.state.AcceptTransaction(t)
	if err != nil {
		if err != consensus.ConflictingTransactionErr {
			// TODO: Change this println to a logging statement.
			fmt.Println("AcceptTransaction Error:", err)
		}
		return
	}

	c.updateMiner(c.miner)

	go c.server.Broadcast("AcceptTransaction", t, nil)
	return
}

// listen waits until a new block or transaction arrives, then attempts to
// process and rebroadcast it.
func (c *Core) listen() {
	for {
		select {
		case b := <-c.blockChan:
			c.processBlock(b)

		case t := <-c.transactionChan:
			c.processTransaction(t)
		}
	}
}
