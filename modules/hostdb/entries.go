package hostdb

import (
	"math"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// TODO: Add a bunch of different ways to arrive at weight, which can each be
// chosen according to the need at hand. This might also require having a bunch
// of different weights at each node in the tree.

// host.Weight() determines the weight of a specific host, which is:
//
//		Freeze * Collateral / square(Price).
//
// Freeze has to be linear, because any non-linear freeze will invite sybil
// attacks.
//
// For now, Collateral is also linear because an increased burn means
// increased risk for the host (Freeze on the other hand has no risk). It
// might be better to make burn grow sublinearly, such as taking
// sqrt(Collateral) or burn^(4/5).
//
// We take the square of the price to heavily emphasize hosts that have a low
// price. This is also a bit simplistic however, because we're not sure what
// the host might be charging for bandwidth.
func entryWeight(entry modules.HostEntry) consensus.Currency {
	// Catch a divide by 0 error, and let all hosts have at least some weight.
	//
	// TODO: Perhaps there's a better way to do this.
	if entry.Price == 0 {
		entry.Price = 1
	}
	if entry.Collateral == 0 {
		entry.Collateral = 1
	}
	if entry.Freeze == 0 {
		entry.Freeze = 1
	}

	adjustedCollateral := float64(entry.Collateral)
	adjustedFreeze := float64(entry.Freeze)
	adjustedPrice := math.Sqrt(float64(entry.Price))

	weight := adjustedFreeze * adjustedCollateral / adjustedPrice
	return consensus.Currency(weight)
}
