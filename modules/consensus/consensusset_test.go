package consensus

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
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

	persistDir string

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

	// Create modules.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
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

		persistDir: testdir,

		csUpdateChan:     cs.ConsensusSetNotify(),
		minerUpdateChan:  m.MinerNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		walletUpdateChan: w.WalletNotify(),
	}
	cst.csUpdateWait()

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
		cst.csUpdateWait()
	}
	return cst, nil
}

// TestNilInputs tries to create new consensus set modules using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestNilInputs")
	_, err := New(nil, testdir)
	if err != ErrNilGateway {
		t.Fatal(err)
	}
}

// TestClosing tries to close a consenuss set.
func TestDatabaseClosing(t *testing.T) {
	testdir := build.TempDir(modules.ConsensusDir, "TestClosing")

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, testdir)
	if err != nil {
		t.Fatal(err)
	}
	err = cs.Close()
	if err != nil {
		t.Error(err)
	}
}
