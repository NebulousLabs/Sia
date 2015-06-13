package wallet

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
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
}

// spendCoins sends the desired amount of coins to the desired address, calling
// wait at all of the appropriate places to assist synchronization.
func (wt *walletTester) spendCoins(amount types.Currency, dest types.UnlockHash) (types.Transaction, error) {
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	id, err := wt.wallet.RegisterTransaction(types.Transaction{})
	if err != nil {
		return types.Transaction{}, err
	}
	_, err = wt.wallet.FundTransaction(id, amount)
	if err != nil {
		return types.Transaction{}, err
	}
	wt.tpUpdateWait()
	_, _, err = wt.wallet.AddSiacoinOutput(id, output)
	if err != nil {
		return types.Transaction{}, err
	}
	txn, err := wt.wallet.SignTransaction(id, true)
	if err != nil {
		return types.Transaction{}, err
	}
	err = wt.tpool.AcceptTransaction(txn)
	if err != nil {
		return types.Transaction{}, err
	}
	wt.tpUpdateWait()
	return txn, nil
}

// csUpdateWait should be called any time that an update is pushed from the
// consensus package. This will keep all of the modules synchronized.
func (wt *walletTester) csUpdateWait() {
	<-wt.csUpdateChan
	wt.tpUpdateWait()
}

// tpUpdateWait should be called any time an update is pushed from the
// transaction pool. This will keep all of the modules synchronized.
func (wt *walletTester) tpUpdateWait() {
	<-wt.tpoolUpdateChan
	<-wt.minerUpdateChan
	<-wt.walletUpdateChan
}

// createWalletTester takes a testing.T and creates a WalletTester.
func createWalletTester(name string) (*walletTester, error) {
	testdir := build.TempDir("wallet", name)

	// Create the modules
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w)
	if err != nil {
		return nil, err
	}

	// Assemble all componenets into a wallet tester.
	wt := &walletTester{
		cs:     cs,
		tpool:  tp,
		miner:  m,
		wallet: w,

		csUpdateChan:     cs.ConsensusSetNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = wt.miner.FindBlock()
		if err != nil {
			return nil, err
		}
		wt.csUpdateWait()
	}

	return wt, nil
}
