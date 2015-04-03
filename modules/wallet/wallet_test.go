package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/types"
)

var (
	walletNum int = 0
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type walletTester struct {
	cs     *consensus.State
	tpool  modules.TransactionPool
	miner  modules.Miner
	wallet *Wallet

	minerChan  <-chan struct{}
	walletChan <-chan struct{}

	t *testing.T
}

// spendCoins sends the desired amount of coins to the desired address, calling
// wait at all of the appropriate places to assist synchronization.
func (wt *walletTester) spendCoins(amount types.Currency, dest types.UnlockHash) (t types.Transaction, err error) {
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	id, err := wt.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	_, err = wt.wallet.FundTransaction(id, amount)
	if err != nil {
		return
	}
	wt.updateWait()
	_, _, err = wt.wallet.AddOutput(id, output)
	if err != nil {
		return
	}
	t, err = wt.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = wt.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}
	wt.updateWait()
	return
}

// updateWait blocks while an update propagates through the modules.
func (wt *walletTester) updateWait() {
	<-wt.minerChan
	<-wt.walletChan
}

// NewWalletTester takes a testing.T and creates a WalletTester.
func NewWalletTester(directory string, t *testing.T) (wt *walletTester) {
	// Create the consensus set.
	cs := consensus.CreateGenesisState()

	// Create the gateway.
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", cs, gDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction pool.
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	wDir := tester.TempDir(directory, modules.WalletDir)
	w, err := New(cs, tp, wDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, g, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble all componenets into a wallet tester.
	wt = &walletTester{
		cs:     cs,
		tpool:  tp,
		miner:  m,
		wallet: w,

		minerChan:  m.MinerSubscribe(),
		walletChan: w.WalletSubscribe(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = wt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		wt.updateWait()
	}

	return
}
