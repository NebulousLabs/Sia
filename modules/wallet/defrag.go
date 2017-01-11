package wallet

import (
	"sort"

	"github.com/NebulousLabs/Sia/types"
)

const (
	// defragThreshold is the number of outputs a wallet is allowed before it is
	// defragmented.
	defragThreshold = 20
)

// defragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) defragWallet() {
	if len(w.siacoinOutputs) < defragThreshold {
		return
	}

	var so sortedOutputs
	for scoid, sco := range w.siacoinOutputs {
		so.ids = append(so.ids, scoid)
		so.outputs = append(so.outputs, sco)
	}
	sort.Sort(sort.Reverse(so))

	defragOutputs := so.outputs[:15]
	var totalOutputValue types.Currency
	for _, output := range defragOutputs {
		totalOutputValue = totalOutputValue.Add(output.Value)
	}
	totalOutputValue = totalOutputValue.Div64(uint64(len(defragOutputs)))

	w.log.Printf("defragmenting wallet: %v outputs, %v total value\n", len(defragOutputs), totalOutputValue)

	var dest types.UnlockHash
	for h := range w.keys {
		dest = h
		break
	}

	go func() {
		_, err := w.SendSiacoins(totalOutputValue, dest)
		if err != nil {
			w.log.Println("error after call to SendSiacoins: ", err)
		}
	}()
}
