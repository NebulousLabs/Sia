package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// sortedOutputs is a struct containing a slice of siacoin outputs and their
// corresponding ids. sortedOutputs can be sorted using the sort package.
type sortedOutputs struct {
	ids     []types.SiacoinOutputID
	outputs []types.SiacoinOutput
}

// ConfirmedBalance returns the balance of the wallet according to all of the
// confirmed transactions.
func (w *Wallet) ConfirmedBalance() (siacoinBalance types.Currency, siafundBalance types.Currency, siafundClaimBalance types.Currency) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, sco := range w.siacoinOutputs {
		siacoinBalance = siacoinBalance.Add(sco.Value)
	}
	w.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSiafundOutputs).ForEach(func(_, sfoBytes []byte) error {
			var sfo types.SiafundOutput
			err := encoding.Unmarshal(sfoBytes, &sfo)
			siafundBalance = siafundBalance.Add(sfo.Value)
			siafundClaimBalance = siafundClaimBalance.Add(w.siafundPool.Sub(sfo.ClaimStart).Mul(sfo.Value).Div(types.SiafundCount))
			return err
		})
	})
	return
}

// UnconfirmedBalance returns the number of outgoing and incoming siacoins in
// the unconfirmed transaction set. Refund outputs are included in this
// reporting.
func (w *Wallet) UnconfirmedBalance() (outgoingSiacoins types.Currency, incomingSiacoins types.Currency) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, upt := range w.unconfirmedProcessedTransactions {
		for _, input := range upt.Inputs {
			if input.FundType == types.SpecifierSiacoinInput && input.WalletAddress {
				outgoingSiacoins = outgoingSiacoins.Add(input.Value)
			}
		}
		for _, output := range upt.Outputs {
			if output.FundType == types.SpecifierSiacoinOutput && output.WalletAddress {
				incomingSiacoins = incomingSiacoins.Add(output.Value)
			}
		}
	}
	return
}

// SendSiacoins creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiacoins(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	if err := w.tg.Add(); err != nil {
		return nil, err
	}
	defer w.tg.Done()

	tpoolFee := types.SiacoinPrecision.Mul64(10) // TODO: better fee algo.
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(amount.Add(tpoolFee))
	if err != nil {
		return nil, err
	}
	txnBuilder.AddMinerFee(tpoolFee)
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return nil, err
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return nil, err
	}
	return txnSet, nil
}

// SendSiafunds creates a transaction sending 'amount' to 'dest'. The transaction
// is submitted to the transaction pool and is also returned.
func (w *Wallet) SendSiafunds(amount types.Currency, dest types.UnlockHash) ([]types.Transaction, error) {
	if err := w.tg.Add(); err != nil {
		return nil, err
	}
	defer w.tg.Done()
	tpoolFee := types.SiacoinPrecision.Mul64(10) // TODO: better fee algo.
	output := types.SiafundOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(tpoolFee)
	if err != nil {
		return nil, err
	}
	err = txnBuilder.FundSiafunds(amount)
	if err != nil {
		return nil, err
	}
	txnBuilder.AddMinerFee(tpoolFee)
	txnBuilder.AddSiafundOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return nil, err
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return nil, err
	}
	return txnSet, nil
}

// Len returns the number of elements in the sortedOutputs struct.
func (so sortedOutputs) Len() int {
	if build.DEBUG && len(so.ids) != len(so.outputs) {
		panic("sortedOutputs object is corrupt")
	}
	return len(so.ids)
}

// Less returns whether element 'i' is less than element 'j'. The currency
// value of each output is used for comparison.
func (so sortedOutputs) Less(i, j int) bool {
	return so.outputs[i].Value.Cmp(so.outputs[j].Value) < 0
}

// Swap swaps two elements in the sortedOutputs set.
func (so sortedOutputs) Swap(i, j int) {
	so.ids[i], so.ids[j] = so.ids[j], so.ids[i]
	so.outputs[i], so.outputs[j] = so.outputs[j], so.outputs[i]
}
