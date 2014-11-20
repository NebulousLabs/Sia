package siad

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Andromeda/siacore"
)

func TestToggleMining(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	state := siacore.CreateGenesisState()
	miner := CreateMiner()

	if state.Height() != 0 {
		t.Error("unexpected genesis height:", state.Height())
	}

	miner.ToggleMining(state, siacore.CoinAddress{})
	time.Sleep(1 * time.Second)
	miner.ToggleMining(state, siacore.CoinAddress{})
	newHeight := state.Height()
	if newHeight == 0 {
		t.Error("height did not increase after mining for a second")
	}
	time.Sleep(1 * time.Second)
	if state.Height() != newHeight {
		t.Error("height still increasing after disabling mining...")
	}
}
