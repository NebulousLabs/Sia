package siacore

import (
	"testing"

	"github.com/NebulousLabs/Andromeda/hash"
)

// The GenesisHash is the value that StateHash() should return on the genesis
// state.
var GenesisHash = hash.Hash{175, 160, 99, 31, 111, 97, 211, 104, 80, 136, 252, 80, 211, 154, 189, 161, 58, 171, 229, 2, 160, 192, 24, 222, 158, 50, 103, 217, 200, 219, 225, 234}

func TestGenesisStateDeterminism(t *testing.T) {
	s := CreateGenesisState()
	if s.StateHash() != GenesisHash {
		t.Error("hash of genesis state does not equal GenesisHash")
	}
}
