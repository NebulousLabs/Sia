package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A tpoolTester contains a consensus tester and a transaction pool, and
// provides a set of helper functions for testing the transaction pool without
// modules that need to use the transaction pool.
//
// updateChan is a channel that will block until the transaction pool posts an
// update. This is useful for synchronizing with updates from the state.
type tpoolTester struct {
	cs     *consensus.State
	tpool  *TransactionPool
	miner  modules.Miner
	wallet modules.Wallet

	tpoolUpdateChan chan struct{}
	minerUpdateChan <-chan struct{}

	t *testing.T
}

// emptyUnlockTransaction creates a transaction with empty UnlockConditions,
// meaning it's trivial to spend the output.
func (tpt *tpoolTester) emptyUnlockTransaction() types.Transaction {
	// Send money to an anyone-can-spend address.
	emptyHash := types.UnlockConditions{}.UnlockHash()
	txn, err := tpt.spendCoins(types.NewCurrency64(1), emptyHash)
	if err != nil {
		tpt.t.Fatal(err)
	}
	outputID := txn.SiacoinOutputID(0)

	// Create a transaction spending the coins.
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: outputID,
			},
		},
		SiacoinOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{
				Value:      types.NewCurrency64(1),
				UnlockHash: emptyHash,
			},
		},
	}

	return txn
}

// updateWait blocks while an update propagates through the modules.
func (tpt *tpoolTester) updateWait() {
	<-tpt.tpoolUpdateChan
	<-tpt.minerUpdateChan
}

// spendCoins sends the desired amount of coins to the desired address, calling
// wait at all of the appropriate places to assist synchronization.
func (tpt *tpoolTester) spendCoins(amount types.Currency, dest types.UnlockHash) (t types.Transaction, err error) {
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	id, err := tpt.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	_, err = tpt.wallet.FundTransaction(id, amount)
	if err != nil {
		return
	}
	tpt.updateWait()
	_, _, err = tpt.wallet.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = tpt.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = tpt.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}
	tpt.updateWait()
	return
}

// CreatetpoolTester initializes a tpoolTester.
func newTpoolTester(directory string, t *testing.T) (tpt *tpoolTester) {
	// Create the consensus set.
	cs := consensus.CreateGenesisState()

	// Create the gateway.
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", cs, gDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction pool.
	tp, err := New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	wDir := tester.TempDir(directory, modules.WalletDir)
	w, err := wallet.New(cs, tp, wDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, g, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe to the updates of the transaction pool.
	tpoolUpdateChan := make(chan struct{}, 1)
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, tpoolUpdateChan)
	tp.mu.Unlock(id)

	// Assebmle all of the objects in to a tpoolTester
	tpt = &tpoolTester{
		cs:     cs,
		tpool:  tp,
		miner:  m,
		wallet: w,

		tpoolUpdateChan: tpoolUpdateChan,
		minerUpdateChan: m.MinerSubscribe(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = tpt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		tpt.updateWait()
	}

	return
}

// TestNewNilInputs tries to trigger a panic with nil inputs.
func TestNewNilInputs(t *testing.T) {
	// Create a consensus set and gateway.
	cs := consensus.CreateGenesisState()
	gDir := tester.TempDir("Transaction Pool - TestNewNilInputs", modules.GatewayDir)
	g, err := gateway.New(":0", cs, gDir)
	if err != nil {
		t.Fatal(err)
	}
	New(nil, nil)
	New(cs, nil)
	New(nil, g)
}
