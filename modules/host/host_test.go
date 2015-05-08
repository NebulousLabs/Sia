package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

type hostTester struct {
	cs      *consensus.State
	gateway modules.Gateway
	miner   modules.Miner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	host *Host

	csUpdateChan     <-chan struct{}
	hostUpdateChan   <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	minerUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}

	t *testing.T
}

// csUpdateWait blocks until a consensus update has propagated to all modules.
func (ht *hostTester) csUpdateWait() {
	<-ht.csUpdateChan
	<-ht.hostUpdateChan
	ht.tpUpdateWait()
}

// tpUpdateWait blocks until a transaction pool update has propagated to all
// modules.
func (ht *hostTester) tpUpdateWait() {
	<-ht.tpoolUpdateChan
	<-ht.minerUpdateChan
	<-ht.walletUpdateChan
}

// CreateHostTester initializes a HostTester.
func CreateHostTester(name string, t *testing.T) *hostTester {
	testdir := build.TempDir(modules.HostDir, name)

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
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Create the host.
	h, err := New(cs, tp, w, ":0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Assemble all objects into a hostTester
	ht := &hostTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		host: h,

		csUpdateChan:     cs.ConsensusSetNotify(),
		hostUpdateChan:   h.HostNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = m.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		ht.csUpdateWait()
	}

	return ht
}
