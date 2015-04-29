package wallet

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/types"
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type walletTester struct {
	cs     *consensus.State
	tpool  modules.TransactionPool
	miner  modules.Miner
	wallet *Wallet

	csUpdateChan     <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	minerUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}

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
	wt.tpUpdateWait()
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
	wt.tpUpdateWait()
	return
}

func (wt *walletTester) csUpdateWait() {
	<-wt.csUpdateChan
	wt.tpUpdateWait()
}

func (wt *walletTester) tpUpdateWait() {
	<-wt.tpoolUpdateChan
	<-wt.minerUpdateChan
	<-wt.walletUpdateChan
}

// NewWalletTester takes a testing.T and creates a WalletTester.
func NewWalletTester(name string, t *testing.T) (wt *walletTester) {
	testdir := tester.TempDir("wallet", name)

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the consensus set.
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction pool.
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble all componenets into a wallet tester.
	wt = &walletTester{
		cs:     cs,
		tpool:  tp,
		miner:  m,
		wallet: w,

		csUpdateChan:     cs.ConsensusSetNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = wt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		wt.csUpdateWait()
	}

	return
}
