package wallet

import (
	"sort"

	"github.com/NebulousLabs/Sia/types"
)

const (
	// defragThreshold is the number of outputs a wallet is allowed before it is
	// defragmented.
	defragThreshold = 20

	// defragBatchSize defines how many outputs are combined during one defrag.
	defragBatchSize = 15
)

// defragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) defragWallet() {
	if len(w.siacoinOutputs) < defragThreshold {
		return
	}

	// grab all the outputs from the wallet and sort them from
	// largest -> smallest
	var so sortedOutputs
	for scoid, sco := range w.siacoinOutputs {
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

	w.log.Printf("defragmenting wallet: %v outputs, %v total value\n", len(defragOutputs), totalOutputValue)

	// grab a new address from the wallet
	addr, err := w.nextPrimarySeedAddress()
	if err != nil {
		w.log.Println("error getting an address for defrag: ", err)
		return
	}

	// send the sum of the outputs to this wallet. This operation is done in a
	// goroutine since defragWallet() is called under lock.
	go func() {
		_, err := w.SendSiacoins(totalOutputValue, addr.UnlockHash())
		if err != nil {
			w.log.Println("error after call to SendSiacoins: ", err)
		}
	}()
}
