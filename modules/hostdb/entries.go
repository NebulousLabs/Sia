package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// TODO: Add a bunch of different ways to arrive at weight, which can each be
// chosen according to the need at hand. This might also require having a bunch
// of different weights at each node in the tree.

// entryWeight determines the weight of a specific host, which is:
//
//		Freeze * Collateral / square(Price).
//
// Freeze has to be linear, because any non-linear freeze will invite sybil
// attacks.
//
// For now, collateral is also linear because an increased collateral means
// increased risk for the host. (Freeze on the other hand has no risk.) It
// might be better to make collateral grow sublinearly, such as taking
// sqrt(collateral) or collateral^(4/5).
//
// We take the square of the price to heavily emphasize hosts that have a low
// price. This is also a bit simplistic however, because we're not sure what
// the host might be charging for bandwidth.
func entryWeight(entry modules.HostEntry) (weight consensus.Currency) {
	// Catch a divide by 0 error, and let all hosts have at least some weight.
	//
	// TODO: Perhaps there's a better way to do this.
	if entry.Price.IsZero() {
		entry.Price = consensus.NewCurrency(1)
	}
	if entry.Collateral.IsZero() {
		entry.Collateral = consensus.NewCurrency(1)
	}
	if entry.Freeze.IsZero() {
		entry.Freeze = consensus.NewCurrency(1)
	}

	// weight := entry.Freeze * entry.Collateral / sqrt(entry.Price)
	weight.Add(entry.Freeze)
	// mathematically, overflow can only occur here
	err := weight.Mul(entry.Collateral)
	if err != nil {
		// TODO: ???
	}
	weight.Div(entry.Price.Sqrt())

	return
}
