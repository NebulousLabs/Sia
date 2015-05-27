package consensus

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// A consensusSetTester is the helper object for consensus set testing,
// including helper modules and methods for controlling synchronization between
// the tester and the modules.
type consensusSetTester struct {
	gateway modules.Gateway
	miner   modules.Miner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	cs *State

	csUpdateChan     <-chan struct{}
	minerUpdateChan  <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}
}

// csUpdateWait blocks until an update to the consensus set has propagated to
// all modules.
func (cst *consensusSetTester) csUpdateWait() {
	<-cst.csUpdateChan
	cst.tpUpdateWait()
}

// tpUpdateWait blocks until an update to the transaction pool has propagated
// to all modules.
func (cst *consensusSetTester) tpUpdateWait() {
	<-cst.tpoolUpdateChan
	<-cst.minerUpdateChan
	<-cst.walletUpdateChan
}

// createConsensusSetTester creates a consensusSetTester that's ready for use.
func createConsensusSetTester(name string) (*consensusSetTester, error) {
	testdir := build.TempDir(modules.ConsensusDir, name)

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	// Create the consensus set.
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	// Create the transaction pool.
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	// Create the wallet.
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	// Create the miner.
	m, err := miner.New(cs, tp, w)
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a consensusSetTester.
	cst := &consensusSetTester{
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		cs: cs,

		csUpdateChan:     cs.ConsensusSetNotify(),
		minerUpdateChan:  m.MinerNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		walletUpdateChan: w.WalletNotify(),
	}
	return cst, nil
}
