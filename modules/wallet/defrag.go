package wallet

import (
	"sort"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// defragThreshold is the number of outputs a wallet is allowed before it is
	// defragmented.
	defragThreshold = 50

	// defragBatchSize defines how many outputs are combined during one defrag.
	defragBatchSize = 40
)

// defragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) defragWallet() {
	// accumulate a map of non-dust outputs
	nonDustOutputs := make(map[types.SiacoinOutputID]types.SiacoinOutput)
	for id, output := range w.siacoinOutputs {
		if output.Value.Cmp(types.SiacoinPrecision) > 0 {
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

	// choose the defragBatchSizeth largest outputs from the wallet and sum them
	defragOutputs := so.outputs[:defragBatchSize]
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
		txns, err := w.SendSiacoins(totalOutputValue, addr.UnlockHash())
		if err != nil {
			w.log.Printf("Attempted to defragment wallet but failed. %v outputs used, %vH total coins. Error: %v.\n ", len(defragOutputs), totalOutputValue, err)
			return
		}

		w.log.Printf("Successfully defragmented wallet. %v outputs used, %vH coins defragmented. Defragment transaction size: %vB.\n", len(defragOutputs), totalOutputValue, len(encoding.Marshal(txns)))
	}()
}
