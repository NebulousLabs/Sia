package sia

import (
	"fmt"

	"github.com/NebulousLabs/Sia/consensus"
)

// BlockChan provides a channel to inform the core of new blocks.
func (c *Core) BlockChan() chan consensus.Block {
	return c.blockChan
}

// TransactionChan provides a channel to inform the core of new transactions.
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

// processBlock locks the state and then attempts to integrate the block.
// Invalid blocks will result in an error.
//
// Mutex note: state mutexes are pretty broken. TODO: Fix this.
func (c *Core) processBlock(b consensus.Block) (err error) {
	c.state.Lock()
	initialStateHeight := c.state.Height()
	rewoundBlocks, appliedBlocks, outputDiffs, err := c.state.AcceptBlock(b)
	c.state.Unlock()
	if err == consensus.BlockKnownErr || err == consensus.KnownOrphanErr {
		return
	} else if err != nil {
		// Call CatchUp() if an unknown orphan is sent.
		if err == consensus.UnknownOrphanErr {
			go c.CatchUp(c.server.RandomPeer())
		}
		return
	}

	err = c.hostDB.Update(initialStateHeight, rewoundBlocks, appliedBlocks)
	if err != nil {
		return
	}
	err = c.wallet.Update(outputDiffs)
	if err != nil {
		return
	}
	err = c.UpdateMiner(c.miner.Threads())
	if err != nil {
		return
	}
	// c.storageProofMaintenance(initialStateHeight, rewoundBlockIDs, appliedBlockIDs)

	// Broadcast all valid blocks.
	go c.server.Broadcast("AcceptBlock", b, nil)
	return
}

// processTransaction locks the state and then attempts to integrate the
// transaction into the state. An error will be returned for invalid or
// duplicate transactions.
//
// Mutex note: state mutexes are pretty broken. TODO: fix
func (c *Core) processTransaction(t consensus.Transaction) (err error) {
	c.state.Lock()
	err = c.state.AcceptTransaction(t)
	c.state.Unlock()
	if err != nil {
		if err != consensus.ConflictingTransactionErr {
			fmt.Println("AcceptTransaction Error:", err)
		}
		return
	}

	c.UpdateMiner(c.miner.Threads())

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
