package siad

import (
	"github.com/NebulousLabs/Andromeda/siacore"
)

// This file is here to provide access to information about the state without
// actually needing to export the state. This allows importing packages to see
// things like state height and depth, but without giving them the ability to
// disrupt the environment's image of the state.

type StateInfo struct {
	Height siacore.BlockHeight
	Target siacore.Target
	Depth  siacore.Target
}

func (e *Environment) StateInfo() StateInfo {
	e.state.Lock()
	defer e.state.Unlock()

	return StateInfo{
		Height: e.state.Height(),
		Target: e.state.CurrentTarget(),
		Depth:  e.state.Depth(),
	}
}
