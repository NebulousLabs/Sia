package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/network"
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
	c.state.RLock()
	defer c.state.RUnlock()

	return StateInfo{
		CurrentBlock: c.state.CurrentBlock().ID(),
		Height:       c.state.Height(),
		Target:       c.state.CurrentTarget(),
		Depth:        c.state.Depth(),
		EarliestLegalTimestamp: c.state.EarliestLegalTimestamp(),
	}
}

func (c *Core) DeepStateInfo() DeepStateInfo {
	c.state.RLock()
	defer c.state.RUnlock()

	return DeepStateInfo{
		StateHash: c.state.StateHash(),

		UtxoSet:         c.state.SortedUtxoSet(),
		TransactionList: c.state.TransactionList(),
	}
}

// Output returns the output that corresponds with a certain OutputID. It does
// not lock the mutex, which means it could potentially (but usually doesn't)
// produce weird or incorrect output.
func (c *Core) Output(id consensus.OutputID) (output consensus.Output, err error) {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.Output(id)
}

func (c *Core) Height() consensus.BlockHeight {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.Height()
}

func (c *Core) TransactionList() []consensus.Transaction {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.TransactionList()
}

func (c *Core) BlockFromID(bid consensus.BlockID) (consensus.Block, error) {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.BlockFromID(bid)
}

func (c *Core) BlockAtHeight(height consensus.BlockHeight) (consensus.Block, error) {
	c.state.RLock()
	defer c.state.RUnlock()
	return c.state.BlockAtHeight(height)
}

////////////////////////
// OBSOLETE FUNCITONS //
////////////////////////

// Some of these functions just need to be moved to other files, some will be
// deleted entirely.

func (c *Core) AddressBook() []network.Address {
	return c.server.AddressBook()
}

func (c *Core) RandomPeer() network.Address {
	return c.server.RandomPeer()
}

func (c *Core) Address() network.Address {
	return c.server.Address()
}
