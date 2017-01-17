package wallet

import (
	"sort"

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

// defragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) defragWallet() {
	// only defrag if the wallet is unlocked
	if !w.unlocked {
		return
	}

	// accumulate a map of non-dust outputs
	nonDustOutputs := make(map[types.SiacoinOutputID]types.SiacoinOutput)
	for id, output := range w.siacoinOutputs {
		if output.Value.Cmp(dustValue()) > 0 {
			nonDustOutputs[id] = output
		}
	}

	// only defrag if there are >defragThreshold non-dust outputs
	if len(nonDustOutputs) < defragThreshold {
		return
	}

	// sort the outputs from largest -> smallest
	var so sortedOutputs
	for scoid, sco := range nonDustOutputs {
		so.ids = append(so.ids, scoid)
		so.outputs = append(so.outputs, sco)
	}
	sort.Sort(sort.Reverse(so))

	// choose the defragBatchSizeth largest outputs, ignoring the first
	// defragStartIndex largest outputs, from the wallet and sum them
	defragOutputs := so.outputs[defragStartIndex : defragStartIndex+defragBatchSize]
	var totalOutputValue types.Currency
	for _, output := range defragOutputs {
		totalOutputValue = totalOutputValue.Add(output.Value)
	}

	// grab a new address from the wallet
	addr, err := w.nextPrimarySeedAddress()
	if err != nil {
		w.log.Println("Error getting an address for defragmentation: ", err)
		return
	}

	// send the sum of the outputs to this wallet. This operation is done in a
	// goroutine since defragWallet() is called under lock.
	go func() {
		tbuilder := w.StartTransaction()
		fee := types.SiacoinPrecision.Mul64(10)

		err := tbuilder.FundSiacoins(fee)
		if err != nil {
			w.log.Println("Error funding fee in defrag transaction: ", err)
			return
		}

		tbuilder.AddMinerFee(fee)

		// here we leverage an implementation detail of the transaction builder.
		// Calling FundSiacoins with the exact value of an existing output will
		// choose that output for use in the transaction. So, here we call
		// FundSiacoins with the value of each output we're consolidating.
		for _, output := range defragOutputs {
			err := tbuilder.FundSiacoins(output.Value)
			if err != nil {
				w.log.Println("Error funding output in defrag transaction: ", err)
				return
			}
		}

		// consolidate the outputs into one output.
		tbuilder.AddSiacoinOutput(types.SiacoinOutput{
			Value:      totalOutputValue,
			UnlockHash: addr.UnlockHash(),
		})

		// sign the transaction set
		txns, err := tbuilder.Sign(true)
		if err != nil {
			w.log.Println("Error signing transaction set in defrag transaction: ", err)
			return
		}

		// accept the transaction set
		err = w.tpool.AcceptTransactionSet(txns)
		if err != nil {
			w.log.Println("Error accepting transaction set in defrag transaction: ", err)
		}
	}()
}
