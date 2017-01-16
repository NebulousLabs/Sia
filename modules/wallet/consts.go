package wallet

import (
	"github.com/NebulousLabs/Sia/types"
)

// dustValue is the quantity below which a Currency is considered to be Dust.
func dustValue() types.Currency {
	return types.SiacoinPrecision
}
