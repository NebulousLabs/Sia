package siacore

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type Miner interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the miner. Info is not relevant to the sia package, but
	// instead to the front end.
	Info() ([]byte, error)

	// SubsidyAddress returns the address that is currently being used by the
	// miner while looking for a block.
	SubsidyAddress() consensus.CoinAddress

	// Update takes a block and a set of transactions. The miner will use the
	// given block as the parent, and will use the transactions as the set of
	// transactions in the block.
	Update(parentID consensus.BlockID, transactionSet []consensus.Transaction, nextTarget consensus.Target, subsidyAddress consensus.CoinAddress, earliestTimestamp consensus.Timestamp) error

	// StartMining takes as input a number of threads to use for mining. 0 will return
	// an error.
	StartMining() error

	// StopMining will take all of the threads down to 0. If mining is already
	// stopped, an error will be returned.
	StopMining() error

	// SolveBlock will attempt to solve a block, returning the most recent
	// attempt and indicating whether the solve was successful or not.
	SolveBlock() (block consensus.Block, solved bool, err error)
}

func (e *Environment) StartMining(threads int) error {
	return e.miner.StartMining(threads)
}

func (e *Environment) StopMining() error {
	return e.miner.StopMining()
}

func (e *Environment) MinerInfo() ([]byte, error) {
	return e.miner.Info()
}

// updateMiner needs to be called with the state read-locked.
func (e *Environment) updateMiner() (err error) {
	recentBlock := e.state.CurrentBlock()
	transactionSet := e.state.TransactionPoolDump()
	target := e.state.CurrentTarget()
	earliestTimestamp := e.state.EarliestLegalTimestamp()

	// Get a new address if the recent block belongs to us, otherwise use the
	// current address.
	address := e.miner.SubsidyAddress()
	if address == recentBlock.MinerAddress {
		address, err = e.wallet.CoinAddress()
		if err != nil {
			return
		}
	}

	// Call update on the miner.
	e.miner.Update(recentBlock.ID(), transactionSet, target, address, earliestTimestamp)
	return
}
