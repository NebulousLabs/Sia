package consensus

import (
	"bytes"

	"github.com/NebulousLabs/Sia/types"
)

// checkMinerPayouts compares a block's miner payouts to the block's subsidy and
// returns true if they are equal.
func checkMinerPayouts(b types.Block, height types.BlockHeight) bool {
	// Add up the payouts and check that all values are legal.
	var payoutSum types.Currency
	for _, payout := range b.MinerPayouts {
		if payout.Value.IsZero() {
			return false
		}
		payoutSum = payoutSum.Add(payout.Value)
	}

	// Compare the payouts to the subsidy.
	subsidy := b.CalculateSubsidy(height)
	if subsidy.Cmp(payoutSum) != 0 {
		return false
	}
	return true
}

// checkTarget returns true if the block's ID meets the given target.
func checkTarget(b types.Block, target types.Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}
