package hostdb

// hostdb_test.go creates the hostdb tester and implements a few helper
// functions for managing hostdb tests.

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// hdbTester is used during testing to initialize a hostdb and useful helper
// modules, and helps to keep them all synchronized. The update channels are
// used for this synchronization. Any time that an update is submitted to the
// consensus set, consensusUpdateWait should be called. Any time that an update
// is submitted to the transaction pool (such as a new transaction),
// tpoolUpdateWait should be called.
type hdbTester struct {
	cs      *consensus.State
	gateway modules.Gateway
	host    modules.Host
	miner   modules.Miner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	hostdb *HostDB

	csUpdateChan     <-chan struct{}
	hostUpdateChan   <-chan struct{}
	hostdbUpdateChan <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	minerUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}

	t *testing.T
}

// csUpdateWait listens on all channels until a consensus set update has
// reached all modules. csUpdateWait should be called every time that there is
// an update to the consensus set (typically only when there is a new block),
// this will keep all of the modules in the hostdb synchronized.
func (hdbt *hdbTester) csUpdateWait() {
	<-hdbt.csUpdateChan
	<-hdbt.hostdbUpdateChan
	<-hdbt.hostUpdateChan
	hdbt.tpUpdateWait()
}

// tpUpdateWait listens on all channels until a transaction pool update has
// reached all modules. tpUpdateWait should be called any time that an update
// is pushed to the transaction pool.
func (hdbt *hdbTester) tpUpdateWait() {
	<-hdbt.tpoolUpdateChan
	<-hdbt.minerUpdateChan
	<-hdbt.walletUpdateChan
}

// newHDBTester returns a ready-to-use hdb tester, with all modules
// initialized.
func newHDBTester(name string, t *testing.T) *hdbTester {
	testdir := tester.TempDir("hostdb", name)

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
	hdb, err := New(cs, g)
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

	// Create the host.
	h, err := host.New(cs, tp, w, filepath.Join(testdir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, g, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assemble all objects into an hdbTester.
	hdbt := &hdbTester{
		cs:      cs,
		gateway: g,
		host:    h,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		hostdb: hdb,

		csUpdateChan:     cs.ConsensusSetNotify(),
		hostUpdateChan:   h.HostNotify(),
		hostdbUpdateChan: hdb.HostDBNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = hdbt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		hdbt.csUpdateWait()
	}

	// TODO: Reconsider the way that the RPC's happen.
	g.RegisterRPC("HostSettings", h.Settings)

	return hdbt
}

// TestNilInputs tries supplying the hostdb with nil inputs can checks for
// correct rejection.
func TestNilInputs(t *testing.T) {
	hdbt := newHDBTester("TestNilInputs", t)
	_, err := New(nil, nil)
	if err == nil {
		t.Error("Should get an error when using nil inputs")
	}
	_, err = New(nil, hdbt.gateway)
	if err != ErrNilConsensusSet {
		t.Error("expecting ErrNilConsensusSet:", err)
	}
	_, err = New(hdbt.cs, nil)
	if err != ErrNilGateway {
		t.Error("expecting ErrNilGateway:", err)
	}
}
