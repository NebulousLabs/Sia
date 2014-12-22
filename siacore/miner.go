package siacore

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type Miner interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the miner. Info is not relevant to the sia package, but
	// instead to the front end.
	Info() ([]byte, error)

	// Update takes a block and a set of transactions. The miner will use the
	// given block as the parent, and will use the transactions as the set of
	// transactions in the block.
	Update(parentID consensus.BlockID, transactionSet []consensus.Transaction, nextTarget consensus.Target, subsidyAddress consensus.CoinAddress, earliestTimestamp consensus.Timestamp) error

	// Mine takes as input a number of threads to use for mining. 0 will return
	// an error.
	StartMining(threads int) error

	// Stop mining will take all of the threads down to 0. If mining is already
	// stopped, an error will be returned.
	StopMining() error
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
