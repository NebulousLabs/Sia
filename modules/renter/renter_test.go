package renter

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// renterTester contains all of the modules that are used while testing the renter.
type renterTester struct {
	cs      *consensus.State
	gateway modules.Gateway
	hostdb  modules.HostDB
	miner   modules.Miner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	renter *Renter

	csUpdateChan     <-chan struct{}
	hostdbUpdateChan <-chan struct{}
	minerUpdateChan  <-chan struct{}
	renterUpdateChan <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}

	t *testing.T
}

// csUpdateWiat waits until a consensus set update has propagated to all
// modules.
func (rt *renterTester) csUpdateWait() {
	<-rt.csUpdateChan
	<-rt.hostdbUpdateChan
	<-rt.renterUpdateChan
	rt.tpUpdateWait()
}

// tpUpdateWait waits until a transaction pool update has propagated to all
// modules.
func (rt *renterTester) tpUpdateWait() {
	<-rt.tpoolUpdateChan
	<-rt.minerUpdateChan
	<-rt.walletUpdateChan
}

// newRenterTester creates a ready-to-use renter tester with money in the
// wallet.
func newRenterTester(name string, t *testing.T) *renterTester {
	testdir := tester.TempDir("renter", name)

	// Create the consensus set.
	cs, err := consensus.New(filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the gateway.
	g, err := gateway.New(":0", cs, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the hostdb.
	hdb, err := hostdb.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the tpool.
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the renter.
	r, err := New(cs, g, hdb, w, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, g, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble all pieces into a renter tester.
	rt := &renterTester{
		cs:      cs,
		gateway: g,
		hostdb:  hdb,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		renter: r,

		csUpdateChan:     cs.ConsensusSetNotify(),
		hostdbUpdateChan: hdb.HostDBNotify(),
		renterUpdateChan: r.RenterNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = rt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		rt.csUpdateWait()
	}
	return rt
}

// TestNilInputs tries supplying the renter with nil inputs and checks for
// correct rejection.
func TestNilInputs(t *testing.T) {
	rt := newRenterTester("TestNilInputs", t)
	_, err := New(nil, nil, nil, nil, modules.RenterDir)
	if err == nil {
		t.Error("no error returned for nil inputs")
	}
	_, err = New(rt.cs, rt.gateway, rt.hostdb, rt.wallet, modules.RenterDir)
	if err != nil {
		t.Error(err)
	}
	_, err = New(nil, rt.gateway, rt.hostdb, rt.wallet, modules.RenterDir)
	if err != ErrNilCS {
		t.Error(err)
	}
	_, err = New(rt.cs, nil, rt.hostdb, rt.wallet, modules.RenterDir)
	if err != ErrNilGateway {
		t.Error(err)
	}
	_, err = New(rt.cs, rt.gateway, nil, rt.wallet, modules.RenterDir)
	if err != ErrNilHostDB {
		t.Error(err)
	}
	_, err = New(rt.cs, rt.gateway, rt.hostdb, nil, modules.RenterDir)
	if err != ErrNilWallet {
		t.Error(err)
	}
}
