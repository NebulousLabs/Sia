package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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

	// ensure durability of reported balance
	w.syncDB()

	dbForEachSiacoinOutput(w.dbTx, func(_ types.SiacoinOutputID, sco types.SiacoinOutput) {
		if sco.Value.Cmp(dustValue()) > 0 {
			siacoinBalance = siacoinBalance.Add(sco.Value)
		}
	})

	siafundPool, err := dbGetSiafundPool(w.dbTx)
	if err != nil {
		return
	}
	dbForEachSiafundOutput(w.dbTx, func(_ types.SiafundOutputID, sfo types.SiafundOutput) {
		siafundBalance = siafundBalance.Add(sfo.Value)
		if sfo.ClaimStart.Cmp(siafundPool) > 0 {
			// Skip claims larger than the siafund pool. This should only
			// occur if the siafund pool has not been initialized yet.
			w.log.Debugf("skipping claim with start value %v because siafund pool is only %v", sfo.ClaimStart, siafundPool)
			return
		}
		siafundClaimBalance = siafundClaimBalance.Add(siafundPool.Sub(sfo.ClaimStart).Mul(sfo.Value).Div(types.SiafundCount))
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
			if output.FundType == types.SpecifierSiacoinOutput && output.WalletAddress && output.Value.Cmp(dustValue()) > 0 {
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
	if !w.unlocked {
		w.log.Println("Attempt to send coins has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}

	_, tpoolFee := w.tpool.FeeEstimation()
	tpoolFee = tpoolFee.Mul64(750) // Estimated transaction size in bytes
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}

	txnBuilder := w.StartTransaction()
	err := txnBuilder.FundSiacoins(amount.Add(tpoolFee))
	if err != nil {
		w.log.Println("Attempt to send coins has failed - failed to fund transaction:", err)
		return nil, build.ExtendErr("unable to fund transaction", err)
	}
	txnBuilder.AddMinerFee(tpoolFee)
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - failed to sign transaction:", err)
		return nil, build.ExtendErr("unable to sign transaction", err)
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - transaction pool rejected transaction:", err)
		return nil, build.ExtendErr("unable to get transaction accepted", err)
	}
	w.log.Println("Submitted a siacoin transfer transaction set for value", amount.HumanString(), "with fees", tpoolFee.HumanString(), "IDs:")
	for _, txn := range txnSet {
		w.log.Println("\t", txn.ID())
	}
	return txnSet, nil
}

// SendSiacoinsMulti creates a transaction that includes the specified
// outputs. The transaction is submitted to the transaction pool and is also
// returned.
func (w *Wallet) SendSiacoinsMulti(outputs []types.SiacoinOutput) ([]types.Transaction, error) {
	if err := w.tg.Add(); err != nil {
		return nil, err
	}
	defer w.tg.Done()
	if !w.unlocked {
		w.log.Println("Attempt to send coins has failed - wallet is locked")
		return nil, modules.ErrLockedWallet
	}

	txnBuilder := w.StartTransaction()

	// Add estimated transaction fee.
	_, tpoolFee := w.tpool.FeeEstimation()
	tpoolFee = tpoolFee.Mul64(2)                              // We don't want send-to-many transactions to fail.
	tpoolFee = tpoolFee.Mul64(1000 + 60*uint64(len(outputs))) // Estimated transaction size in bytes
	txnBuilder.AddMinerFee(tpoolFee)

	// Calculate total cost to wallet.
	// NOTE: we only want to call FundSiacoins once; that way, it will
	// (ideally) fund the entire transaction with a single input, instead of
	// many smaller ones.
	totalCost := tpoolFee
	for _, sco := range outputs {
		totalCost = totalCost.Add(sco.Value)
	}
	err := txnBuilder.FundSiacoins(totalCost)
	if err != nil {
		return nil, build.ExtendErr("unable to fund transaction", err)
	}

	for _, sco := range outputs {
		txnBuilder.AddSiacoinOutput(sco)
	}

	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - failed to sign transaction:", err)
		return nil, build.ExtendErr("unable to sign transaction", err)
	}
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("Attempt to send coins has failed - transaction pool rejected transaction:", err)
		return nil, build.ExtendErr("unable to get transaction accepted", err)
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
	if !w.unlocked {
		return nil, modules.ErrLockedWallet
	}

	_, tpoolFee := w.tpool.FeeEstimation()
	tpoolFee = tpoolFee.Mul64(750) // Estimated transaction size in bytes
	tpoolFee = tpoolFee.Mul64(5)   // use large fee to ensure siafund transactions are selected by miners
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
	w.log.Println("Submitted a siafund transfer transaction set for value", amount.HumanString(), "with fees", tpoolFee.HumanString(), "IDs:")
	for _, txn := range txnSet {
		w.log.Println("\t", txn.ID())
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
