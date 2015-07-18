package transactionpool

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A tpoolTester is used during testing to initialize a transaction pool and
// useful helper modules. The update channels are used to synchronize updates
// that occur during testing. Any time that an update is submitted to the
// transaction pool or consensus set, updateWait() should be called or
// desynchronization could be introduced.
type tpoolTester struct {
	cs      *consensus.ConsensusSet
	gateway modules.Gateway
	tpool   *TransactionPool
	miner   modules.Miner
	wallet  modules.Wallet

	t *testing.T
}

// newTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized.
func newTpoolTester(name string, t *testing.T) *tpoolTester {
	testdir := build.TempDir("transactionpool", name)

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
	tp, err := New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Assebmle all of the objects in to a tpoolTester
	tpt := &tpoolTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		t: t,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := tpt.miner.FindBlock()
		err = tpt.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	return tpt
}

// TestNewNilInputs tries to trigger a panic with nil inputs.
func TestNewNilInputs(t *testing.T) {
	testdir := build.TempDir("transactionpool", "TestNewNilInputs")

	// Create a gateway and consensus set.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	New(nil, nil)
	New(cs, nil)
	New(nil, g)
}
