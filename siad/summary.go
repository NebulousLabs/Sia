package siad

import (
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// This file is here to provide access to information about the state without
// actually needing to export the state. This allows importing packages to see
// things like state height and depth, but without giving them the ability to
// disrupt the environment's image of the state.

type StateInfo struct {
	StateHash hash.Hash

	CurrentBlock           siacore.BlockID
	Height                 siacore.BlockHeight
	Target                 siacore.Target
	Depth                  siacore.Target
	EarliestLegalTimestamp siacore.Timestamp

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
	return StateInfo{
		StateHash: e.state.StateHash(),

		CurrentBlock: e.state.CurrentBlock().ID(),
		Height:       e.state.Height(),
		Target:       e.state.CurrentTarget(),
		Depth:        e.state.Depth(),
		EarliestLegalTimestamp: e.state.EarliestLegalTimestamp(),

		UtxoSet:         e.state.SortedUtxoSet(),
		TransactionList: e.state.TransactionList(),
	}
}

// SafeStateInfo locks the state before doing any reads, ensuring that the
// reads are accurate and not prone to race conditions. This function can
// sometimes take a while to return, however.
func (e *Environment) SafeStateInfo() StateInfo {
	e.state.Lock()
	defer e.state.Unlock()
	return e.StateInfo()
}

// Output returns the output that corresponds with a certain OutputID. It does
// not lock the mutex, which means it could potentially (but usually doesn't)
// produce weird or incorrect output.
func (e *Environment) Output(id siacore.OutputID) (output siacore.Output, err error) {
	return e.state.Output(id)
}

// SafeOutput returns the output that corresponds with a certain OutputID,
// using a mutex when accessing the state.
func (e *Environment) SafeOutput(id siacore.OutputID) (output siacore.Output, err error) {
	e.state.Lock()
	defer e.state.Unlock()
	return e.Output(id)
}

func (e *Environment) Height() siacore.BlockHeight {
	e.state.Lock()
	defer e.state.Unlock()
	return e.state.Height()
}

func (e *Environment) TransactionList() []siacore.Transaction {
	e.state.Lock()
	defer e.state.Unlock()
	return e.state.TransactionList()
}
