package wallet

import (
	"github.com/NebulousLabs/Sia/types"
)

var (
	// DustValue is the value below which a types.Currency is considered Dust.
	dustValue = func() types.Currency {
		return types.SiacoinPrecision
	}()
)
