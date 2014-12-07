package main

import (
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// This file is here to provide access to information about the state without
// actually needing to export the state. This allows importing packages to see
// things like state height and depth, but without giving them the ability to
// disrupt the environment's image of the state.

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	StateHash hash.Hash

	CurrentBlock           siacore.BlockID
	Height                 siacore.BlockHeight
	Target                 siacore.Target
	Depth                  siacore.Target
	EarliestLegalTimestamp siacore.Timestamp
}

// Contains in depth information about the state - potentially a lot of
// information.
type DeepStateInfo struct {
	UtxoSet         []siacore.OutputID
	TransactionList []siacore.Transaction
}

// StateInfo returns a bunch of useful information about the state, doing
// read-only accesses. StateInfo does not lock the state mutex, which means
// that the data could potentially be weird on account of race conditions.
// Because it's just a read-only call, it will not adversely affect the state.
// If accurate data is paramount, SafeStateInfo() should be called, though this
// can adversely affect performance.
func (e *Environment) StateInfo() StateInfo {
	e.state.RLock()
	defer e.state.RUnlock()

	return StateInfo{
		StateHash: e.state.StateHash(),

		CurrentBlock: e.state.CurrentBlock().ID(),
		Height:       e.state.Height(),
		Target:       e.state.CurrentTarget(),
		Depth:        e.state.Depth(),
		EarliestLegalTimestamp: e.state.EarliestLegalTimestamp(),
	}
}

func (e *Environment) DeepStateInfo() DeepStateInfo {
	e.state.RLock()
	defer e.state.RUnlock()

	return DeepStateInfo{
		UtxoSet:         e.state.SortedUtxoSet(),
		TransactionList: e.state.TransactionList(),
	}
}

// Output returns the output that corresponds with a certain OutputID. It does
// not lock the mutex, which means it could potentially (but usually doesn't)
// produce weird or incorrect output.
func (e *Environment) Output(id siacore.OutputID) (output siacore.Output, err error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.Output(id)
}

func (e *Environment) Height() siacore.BlockHeight {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.Height()
}

func (e *Environment) TransactionList() []siacore.Transaction {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.TransactionList()
}

func (e *Environment) BlockFromID(bid siacore.BlockID) (siacore.Block, error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.BlockFromID(bid)
}

func (e *Environment) BlockAtHeight(height siacore.BlockHeight) (siacore.Block, error) {
	e.state.RLock()
	defer e.state.RUnlock()
	return e.state.BlockAtHeight(height)
}

func (e *Environment) AddressBook() []network.NetAddress {
	return e.server.AddressBook()
}

func (e *Environment) RandomPeer() network.NetAddress {
	return e.server.RandomPeer()
}

func (e *Environment) NetAddress() network.NetAddress {
	return e.server.NetAddress()
}
