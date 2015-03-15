package tester

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// CreateTestingConsensusSet creates a ready-to-go consensus set. The
// identifier indicates a prefix that any files created should use.
func CreateTestingConsensusSet(directory string) (state *consensus.State, err error) {
	state = consensus.CreateGenesisState()
	return
}
