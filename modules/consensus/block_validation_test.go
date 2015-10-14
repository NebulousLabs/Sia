package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestCheckMinerPayouts probes the checkMinerPayouts function.
func TestCheckMinerPayouts(t *testing.T) {
	// All tests are done at height = 0.
	coinbase := types.CalculateCoinbase(0)

	// Create a block with a single valid payout.
	b := types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
		},
	}
	if !checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}

	// Try a block with an incorrect payout.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase.Sub(types.NewCurrency64(1))},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is a too-small payout")
	}

	// Try a block with 2 payouts.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase.Sub(types.NewCurrency64(1))},
			{Value: types.NewCurrency64(1)},
		},
	}
	if !checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there are 2 payouts")
	}

	// Try a block with 2 payouts that are too large.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
			{Value: coinbase},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there are two large payouts")
	}

	// Create a block with an empty payout.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
			{},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}
}

// TestCheckTarget probes the checkTarget function.
func TestCheckTarget(t *testing.T) {
	var b types.Block
	lowTarget := types.RootDepth
	highTarget := types.Target{}
	sameTarget := types.Target(b.ID())

	if !checkTarget(b, lowTarget) {
		t.Error("CheckTarget failed for a low target")
	}
	if checkTarget(b, highTarget) {
		t.Error("CheckTarget passed for a high target")
	}
	if !checkTarget(b, sameTarget) {
		t.Error("CheckTarget failed for a same target")
	}
}
