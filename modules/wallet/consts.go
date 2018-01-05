package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// defragBatchSize defines how many outputs are combined during one defrag.
	defragBatchSize = 35

	// defragStartIndex is the number of outputs to skip over when performing a
	// defrag.
	defragStartIndex = 10

	// defragThreshold is the number of outputs a wallet is allowed before it is
	// defragmented.
	defragThreshold = 50

	// rebroadcastInterval is the number of blocks the wallet will wait until
	// it rebroadcasts an unconfirmed transaction by adding it to the
	// transaction pool again.
	rebroadcastInterval = 6

	// RespendTimeout records the number of blocks that the wallet will wait
	// before spending an output that has been spent in the past. If the
	// transaction spending the output has not made it to the transaction pool
	// after the limit, the assumption is that it never will.
	respendTimeout = 72
)

var (
	// rebroadcastTimeout is the amount of blocks after which we stop trying to
	// rebroadcast transactions. The reason why we can't just use
	// respendTimeout as the rebroadcastTimeout is, that the transaction pool
	// will boot transactions after MaxTxnAge. We need to make sure that we
	// leave at least MaxTxnAge blocks after the last broadcast to allow for
	// the transasction to be pruned before the wallet tries to respend it.
	rebroadcastTimeout = types.BlockHeight(respendTimeout - modules.MaxTxnAge)

	// lookaheadBuffer together with lookaheadRescanThreshold defines the constant part
	// of the maxLookahead
	lookaheadBuffer = build.Select(build.Var{
		Dev:      uint64(400),
		Standard: uint64(4000),
		Testing:  uint64(40),
	}).(uint64)

	// lookaheadRescanThreshold is the number of keys in the lookahead that will be
	// generated before a complete wallet rescan is initialized.
	lookaheadRescanThreshold = build.Select(build.Var{
		Dev:      uint64(100),
		Standard: uint64(1000),
		Testing:  uint64(10),
	}).(uint64)
)

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
