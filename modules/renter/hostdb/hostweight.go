package hostdb

import (
	"math/big"

	"github.com/NebulousLabs/Sia/types"
)

var (
	// Because most weights would otherwise be fractional, we set the base
	// weight to 10^80 to give ourselves lots of precision when determing the
	// weight of a host
	baseWeight = types.NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(150), nil))
)

// calculateHostWeight returns the weight of a host according to the settings of
// the host database entry. Currently, only the price is considered.
func calculateHostWeight(entry hostEntry) (weight types.Currency) {
	// If the price is 0, just return the base weight to avoid divide by zero.
	price := entry.Price
	if price.IsZero() {
		return baseWeight
	}

	// Divide the base weight by the price to the fifth power.
	return baseWeight.Div(price).Div(price).Div(price).Div(price).Div(price)
}
