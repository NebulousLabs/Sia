package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
)

// This file is here to provide access to information about the state without
// actually needing to export the state. This allows importing packages to see
// things like state height and depth, but without giving them the ability to
// disrupt the environment's image of the state.

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock           consensus.BlockID
	Height                 consensus.BlockHeight
	Target                 consensus.Target
	Depth                  consensus.Target
	EarliestLegalTimestamp consensus.Timestamp
}

// Contains in depth information about the state - potentially a lot of
// information.
type DeepStateInfo struct {
	StateHash hash.Hash

	UtxoSet         []consensus.Output
	TransactionList []consensus.Transaction
}

// StateInfo returns a bunch of useful information about the state, doing
// read-only accesses. StateInfo does not lock the state mutex, which means
// that the data could potentially be weird on account of race conditions.
// Because it's just a read-only call, it will not adversely affect the state.
// If accurate data is paramount, SafeStateInfo() should be called, though this
// can adversely affect performance.
func (c *Core) StateInfo() StateInfo {
	return StateInfo{
		CurrentBlock: c.state.CurrentBlock().ID(),
		Height:       c.state.Height(),
		Target:       c.state.CurrentTarget(),
		Depth:        c.state.Depth(),
		EarliestLegalTimestamp: c.state.EarliestTimestamp(),
	}
}

func (c *Core) DeepStateInfo() DeepStateInfo {
	return DeepStateInfo{
		StateHash: c.state.StateHash(),

		UtxoSet:         c.state.SortedUtxoSet(),
		TransactionList: c.state.TransactionPoolDump(),
	}
}

// Output returns the output that corresponds with a certain OutputID. It does
// not lock the mutex, which means it could potentially (but usually doesn't)
// produce weird or incorrect output.
func (c *Core) Output(id consensus.OutputID) (output consensus.Output, err error) {
	return c.state.Output(id)
}

func (c *Core) Height() consensus.BlockHeight {
	return c.state.Height()
}

func (c *Core) TransactionPoolDump() []consensus.Transaction {
	return c.state.TransactionPoolDump()
}

func (c *Core) BlockFromID(bid consensus.BlockID) (consensus.Block, error) {
	return c.state.BlockFromID(bid)
}

func (c *Core) BlockAtHeight(height consensus.BlockHeight) (consensus.Block, error) {
	return c.state.BlockAtHeight(height)
}

func (c *Core) StorageProofSegmentIndex(contractID consensus.ContractID, windowIndex consensus.BlockHeight) (index uint64, err error) {
	return c.state.StorageProofSegmentIndex(contractID, windowIndex)
}
