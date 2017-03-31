package wallet

import (
	"github.com/NebulousLabs/Sia/build"
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
//
// TODO: These need to be functions of the wallet that interact with the
// transaction pool.
func dustValue() types.Currency {
	return types.SiacoinPrecision.Mul64(3)
}

// defragFee is the miner fee paid to miners when performing a defrag
// transaction.
//
// TODO: These need to be functions of the wallet that interact with the
// transaction pool.
func defragFee() types.Currency {
	// 35 outputs at an estimated 250 bytes needed per output means about a 10kb
	// total transaction, much larger than your average transaction. So you need
	// a lot of fees.
	return types.SiacoinPrecision.Mul64(20)
}

func init() {
	// Sanity check - the defrag threshold needs to be higher than the batch
	// size plus the start index.
	if build.DEBUG && defragThreshold <= defragBatchSize+defragStartIndex {
		panic("constants are incorrect, defragThreshold needs to be larger than the sum of defragBatchSize and defragStartIndex")
	}
}
