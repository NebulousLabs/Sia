package wallet

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// defragThreshold is the number of outputs a wallet is allowed before it is
	// defragmented.
	defragThreshold = 50

	// defragBatchSize defines how many outputs are combined during one defrag.
	defragBatchSize = 35

	// defragStartIndex is the number of outputs to skip over when performing a
	// defrag.
	defragStartIndex = 10
)

// dustValue is the quantity below which a Currency is considered to be Dust.
func dustValue() types.Currency {
	return types.SiacoinPrecision
}

// defragFee is the miner fee paid to miners when performing a defrag
// transaction.
func defragFee() types.Currency {
	fee := types.SiacoinPrecision.Mul64(5)
	if dustValue().Mul64(defragBatchSize).Cmp(fee) <= 0 {
		return dustValue().Mul64(defragBatchSize)
	}
	return fee
}
