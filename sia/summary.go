package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
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
