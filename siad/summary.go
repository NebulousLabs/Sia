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

func (e *Environment) StateInfo() StateInfo {
	return StateInfo{
		StateHash: e.state.StateHash(),

		CurrentBlock: e.state.CurrentBlock().ID(),
		Height:       e.state.Height(),
		Target:       e.state.CurrentTarget(),
		Depth:        e.state.Depth(),
		EarliestLegalTimestamp: e.state.CurrentEarliestLegalTimestamp(),

		UtxoSet:         e.state.SortedUtxoSet(),
		TransactionList: e.state.TransactionList(),
	}
}

func (e *Environment) Output(id siacore.OutputID) (output siacore.Output, err error) {
	return e.state.Output(id)
}
