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

var (
	// lookaheadRescanThreshold is the number of keys in the lookahead that will be
	// generated before a complete wallet rescan is initialized.
	lookaheadRescanThreshold = build.Select(build.Var{
		Dev:      uint64(100),
		Standard: uint64(1000),
		Testing:  uint64(10),
	}).(uint64)

	// lookaheadBuffer together with lookaheadRescanThreshold defines the constant part
	// of the maxLookahead
	lookaheadBuffer = build.Select(build.Var{
		Dev:      uint64(400),
		Standard: uint64(4000),
		Testing:  uint64(40),
	}).(uint64)
)

// dustValue is the quantity below which a Currency is considered to be Dust.
//
// TODO: These need to be functions of the wallet that interact with the
// transaction pool.
func dustValue() types.Currency {
	return types.SiacoinPrecision
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
	return types.SiacoinPrecision.Mul64(10)
}

func init() {
	// Sanity check - the defrag threshold needs to be higher than the batch
	// size plus the start index.
	if build.DEBUG && defragThreshold <= defragBatchSize+defragStartIndex {
		panic("constants are incorrect, defragThreshold needs to be larger than the sum of defragBatchSize and defragStartIndex")
	}
}

// maxLookahead returns the size of the lookahead for a given seed progress
// which usually is the current primarySeedProgress
func maxLookahead(start uint64) uint64 {
	return start + lookaheadRescanThreshold + lookaheadBuffer + start/10
}
