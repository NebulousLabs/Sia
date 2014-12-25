package sia

import (
	"fmt"

	"github.com/NebulousLabs/Sia/consensus"
)

// BlockChan returns a channel down which blocks can be thrown.
func (e *Environment) BlockChan() chan consensus.Block {
	return e.blockChan
}

func (e *Environment) TransactionChan() chan consensus.Transaction {
	return e.transactionChan
}

// AcceptBlock sends the input block down a channel, where it will be dealt
// with by the Environment's listener.
func (e *Environment) AcceptBlock(b consensus.Block) error {
	e.blockChan <- b
	return nil
}

// AcceptTransaction sends the input transaction down a channel, where it will
// be dealt with by the Environment's listener.
func (e *Environment) AcceptTransaction(t consensus.Transaction) error {
	e.transactionChan <- t
	return nil
}

// processBlock is called by the environment's listener.
func (e *Environment) processBlock(b consensus.Block) (err error) {
	e.state.Lock()
	e.hostDatabase.Lock()
	e.host.Lock()
	defer e.state.Unlock()
	defer e.hostDatabase.Unlock()
	defer e.host.Unlock()

	initialStateHeight := e.state.Height()
	rewoundBlockIDs, appliedBlockIDs, outputDiffs, err := e.state.AcceptBlock(b)
	if err == consensus.BlockKnownErr || err == consensus.KnownOrphanErr {
		return
	} else if err != nil {
		// Call CatchUp() if an unknown orphan is sent.
		if err == consensus.UnknownOrphanErr {
			go e.CatchUp(e.server.RandomPeer())
		}
		return
	}

	err = e.wallet.Update(outputDiffs)
	if err != nil {
		return
	}
	err = e.updateMiner(e.miner)
	if err != nil {
		return
	}
	e.updateHostDB(rewoundBlockIDs, appliedBlockIDs)
	e.storageProofMaintenance(initialStateHeight, rewoundBlockIDs, appliedBlockIDs)

	// Broadcast all valid blocks.
	go e.server.Broadcast("AcceptBlock", b, nil)
	return
}

// processTransaction sends a transaction to the state.
func (e *Environment) processTransaction(t consensus.Transaction) (err error) {
	e.state.Lock()
	defer e.state.Unlock()

	err = e.state.AcceptTransaction(t)
	if err != nil {
		if err != consensus.ConflictingTransactionErr {
			// TODO: Change this println to a logging statement.
			fmt.Println("AcceptTransaction Error:", err)
		}
		return
	}

	e.updateMiner(e.miner)

	go e.server.Broadcast("AcceptTransaction", t, nil)
	return
}

// listen waits until a new block or transaction arrives, then attempts to
// process and rebroadcast it.
func (e *Environment) listen() {
	for {
		select {
		case b := <-e.blockChan:
			e.processBlock(b)

		case t := <-e.transactionChan:
			e.processTransaction(t)
		}
	}
}
